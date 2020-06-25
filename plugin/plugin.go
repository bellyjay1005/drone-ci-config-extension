package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
	"github.com/drone/drone-go/drone"
	"github.com/drone/drone-go/plugin/config"
	"github.com/drone/drone-yaml/yaml"
	"github.com/sirupsen/logrus"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

// Plugin holds the GitHub client, server and token.
type Plugin struct {
	server        string
	token         string
	apiToken      string
	host          string
	auth0Endpoint string
	authEndpoint  string
	apiEndpoint   string
	apiEndpointQA string
	ssm           ssmiface.SSMAPI
	client        *github.Client
}

// APIResponse is the structure for Auth0 API responses
type APIResponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// AuthRequest is the structure for auth API requests
type AuthRequest struct {
	Input struct {
		Accounts []string `json:"accounts"`
		Repo     string   `json:"repo"`
	} `json:"input"`
}

// AuthResponse is the structure for auth API responses
type AuthResponse struct {
	Result     bool   `json:"result"`
	DecisionID string `json:"decision_id"`
}

// TokenData holds an array of strings for token state
type TokenData []string

// TokenState respresents a namespace and information about a token
type TokenState struct {
	Namespace string      `json:"namespace"`
	Data      []TokenData `json:"data"`
}

// New returns a new permission check plugin.
func New(
	server, token, apiToken, host, auth0Endpoint, authEndpoint, apiEndpoint, apiEndpointQA string,
	ssm ssmiface.SSMAPI, client *github.Client) *Plugin {
	return &Plugin{
		server:        server,
		token:         token,
		apiToken:      apiToken,
		host:          host,
		auth0Endpoint: auth0Endpoint,
		authEndpoint:  authEndpoint,
		apiEndpoint:   apiEndpoint,
		apiEndpointQA: apiEndpointQA,
		ssm:           ssm,
		client:        client,
	}
}

// GetAuth0APIKey returns a bearer token with the user data embeded
func (p *Plugin) GetAuth0APIKey(req *config.Request, env string) (string, error) {
	user := req.Build.Sender

	url := p.auth0Endpoint

	withDecryption := true
	path := fmt.Sprintf("/%s/auth0/", env)

	params, _ := p.ssm.GetParametersByPath(&ssm.GetParametersByPathInput{
		Path:           &path,
		WithDecryption: &withDecryption,
	})
	var secret *string
	var id *string
	if params == nil {
		return "", fmt.Errorf("No parameters found under the path %s", path)
	}
	for _, param := range params.Parameters {
		logrus.Debugf("Param and name: %v, %s", param, *param.Name)
		if strings.HasSuffix(*param.Name, "client-id") {
			id = param.Value
		}
		if strings.HasSuffix(*param.Name, "client-secret") {
			secret = param.Value
		}
	}
	if id == nil {
		logrus.Errorf("Client id not found")
		return "", errors.New("Client ID not found")
	}
	if secret == nil {
		logrus.Errorf("Client secret not found")
		return "", errors.New("Client secret not found")
	}

	audience := p.apiEndpoint
	if env == "qa" {
		audience = p.apiEndpointQA
	}
	link := strings.Replace(req.Repo.Link, "https://", "", 1)

	payload, err := GeneratePayload(*id, *secret, audience, user, link)
	logrus.Debugf("Payload %s", payload)
	if err != nil {
		logrus.Errorf("Unable to marshal token state")
		return "", err
	}

	payloadReader := strings.NewReader(payload)
	areq, _ := http.NewRequest("POST", url, payloadReader)

	areq.Header.Add("content-type", "application/json")

	res, _ := http.DefaultClient.Do(areq)

	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)

	logrus.Debug(res)

	var apiRes APIResponse
	logrus.Debug(string(body))
	json.Unmarshal(body, &apiRes)
	logrus.Debug(apiRes)
	return apiRes.AccessToken, nil
}

// GeneratePayload generates a payload for Auth0
func GeneratePayload(id string, secret string, audience string, user string, link string) (string, error) {
	state := TokenState{
		Namespace: "demo",
		Data: []TokenData{
			{"repository", link},
			{"sender", user},
		},
	}
	marshalledState, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	logrus.Debugf("Marshalled State: %s", string(marshalledState))
	escapedState := strconv.Quote(string(marshalledState))
	payload := fmt.Sprintf(`{"client_id":"%s","client_secret":"%s","audience":"%s","grant_type":"client_credentials","drone_username":"%s","state":%s}`, id, secret, audience, user, escapedState)
	return payload, nil
}

// EncryptData uses the drone api to encrypt a string
func (p *Plugin) EncryptData(content string, req *config.Request) (string, error) {
	// create an http client with oauth authentication.
	config := new(oauth2.Config)
	auther := config.Client(
		oauth2.NoContext,
		&oauth2.Token{
			AccessToken: p.apiToken,
		},
	)

	// create the drone client with authenticator
	client := drone.NewClient(p.host, auther)

	// encrypt the data
	secret := &drone.Secret{
		Namespace: req.Repo.Namespace,
		Name:      req.Repo.Name,
		Data:      content,
	}
	encrypted, err := client.Encrypt(req.Repo.Namespace, req.Repo.Name, secret)
	if err != nil {
		logrus.Debugf("Error encrypting: %v", err)
		return "", err
	}
	return encrypted, nil
}

func injectSteps(pipe *yaml.Pipeline, secretName string) {
	// Add the secret to each environment
	for _, step := range pipe.Steps {
		if step.Environment == nil {
			step.Environment = map[string]*yaml.Variable{}
		}
		step.Environment[secretName] = &yaml.Variable{Secret: secretName}
	}
}

// InjectKey takes in a .drone.yml file and inserts an api token for the given env
func (p *Plugin) InjectKey(content string, req *config.Request, env string) (string, string, error) {
	// Get the key to inject into the .drone.yml
	key, err := p.GetAuth0APIKey(req, env)
	if err != nil {
		logrus.Errorf("Error fetching auth0 api key: %v", err)
		return "", "", err
	}

	// Encrypt the key using the drone api
	enc, err := p.EncryptData(key, req)
	if err != nil {
		logrus.Debugf("Error encrypting: %s", err)
		return content, "", err
	}

	// Parse the .drone.yml file
	manifest, err := yaml.Parse(strings.NewReader(content))
	if err != nil {
		logrus.Errorf("Error parsing drone config: %s", err)
		return "", "", err
	}

	secretName := "DEMO_API_TOKEN"
	if env != "pr" {
		secretName = fmt.Sprintf("%s_%s", secretName, strings.ToUpper(env))
	}
	hasPipes := false
	// Find the pipeline in the manifest
	for _, r := range manifest.Resources {
		v, ok := r.(*yaml.Pipeline)
		if !ok {
			continue
		}
		hasPipes = true
		injectSteps(v, secretName)
	}
	if hasPipes == false {
		logrus.Errorf("Pipeline not found in config file")
		return "", "", errors.New("Pipeline not found in config file")
	}
	// Add the secret to the manifest
	manifest.Resources = append(manifest.Resources, &yaml.Secret{
		Kind: "secret",
		Name: secretName,
		Data: enc,
	})

	// Since the manifest was pulled from a file, encoding will not have errors
	newContent, _ := manifest.Encode()
	// content = strings.Replace(string(newContent), "  ", "\t", -1)
	content = fmt.Sprintf("---\n%s", string(newContent))
	return string(content), key, nil
}

// GetGithubFile downloads a specific file from GitHub.
func (p *Plugin) GetGithubFile(ctx context.Context, req *config.Request, org, name, file string) (string, error) {
	opts := &github.RepositoryContentGetOptions{Ref: req.Build.After}
	data, _, _, err := p.client.Repositories.GetContents(ctx, org, name, file, opts)
	if err != nil {
		return "error", err
	}

	// if there is no error and no content, a nil value is
	// returned. The plugin responds with a 204 No Content,
	// instrucing Drone to fallback to a .drone.yml file.
	if data == nil {
		return "", nil
	}

	content, err := data.GetContent()
	if err != nil {
		return "bad data", err
	}

	return content, nil
}

// Validate will validate the .strithon.yml file for the given user
func (p *Plugin) Validate(ctx context.Context, req *config.Request, token string) ([]string, error) {

	// get the .strithon.yml file from the github repository
	content, err := p.GetGithubFile(ctx, req, req.Repo.Namespace, req.Repo.Name, ".strithon.yml")
	if err != nil {
		return nil, err
	}
	if content == "" {
		return nil, nil
	}

	// parse the .strithon.yml file
	bellyjay1005Config, err := ParsestrithonYml(content)
	if err != nil {
		logrus.Debugf("Error parsing the .strithon.yml file: %v", err)
		return nil, err
	}

	// get the list of aws accounts from the .strithon.yml file, no duplicates
	acctMap := make(map[string]bool)
	for _, env := range bellyjay1005Config.Metadata.Environments {
		acctMap[string(env.Account)] = true
	}
	accts := []string{}
	for k := range acctMap {
		accts = append(accts, k)
	}

	// see if the accounts are allowed with the auth api
	if len(accts) > 0 {
		payloadContent := fmt.Sprintf(`{"input":{"accounts":["%v"],"repo":"%s"}}`, strings.Join(accts, `","`), GetRepoLink(req.Repo.Link))
		payload := strings.NewReader(payloadContent)
		logrus.Debugf("Payload to the auth api: %s", payloadContent)
		logrus.Debugf("Auth endpoint: %s", p.authEndpoint)
		request, _ := http.NewRequest("POST", p.authEndpoint+"/v1/data/demo/drone/allow", payload)

		request.Header.Add("Content-Type", "application/json")
		request.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))

		res, err := http.DefaultClient.Do(request)
		if err != nil {
			logrus.Errorf("Error calling auth api: %v", err)
			return nil, err
		}
		defer res.Body.Close()
		body, _ := ioutil.ReadAll(res.Body)

		logrus.Debugf("Auth api response: %v", res)

		var authRes AuthResponse
		logrus.Debug(string(body))
		err = json.Unmarshal(body, &authRes)
		if err != nil {
			logrus.Errorf("Error unmarshalling %s: %s\n\n", string(body), err)
			return nil, err
		}
		logrus.Debugf("Auth response: %v", authRes)
		if !authRes.Result {
			return accts, nil
		}
	}
	return []string{}, nil
}

func injectWarnings(pipe *yaml.Pipeline, repo string, org string, accounts []string) error {
	for _, step := range pipe.Steps {
		if strings.Contains(step.Image, "plugin") {
			step.Image = "alpine"
			step.Name = fmt.Sprintf("%s-unauthorized", step.Name)
			step.Commands = []string{fmt.Sprintf("%s/%s is unauthorized to deploy to accounts %v. Update permissions in https://github.com/bellyjay1005/aws-drone-policy.", org, repo, accounts)}
		}
	}
	return nil
}

func (p *Plugin) replaceWarnings(content string, repo string, org string, accounts []string) (string, error) {
	manifest, err := yaml.Parse(strings.NewReader(content))
	if err != nil {
		logrus.Errorf("Error parsing drone config: %s", err)
		return "", err
	}
	hasPipes := false
	for _, r := range manifest.Resources {
		v, ok := r.(*yaml.Pipeline)
		if !ok {
			continue
		}
		hasPipes = true
		injectWarnings(v, repo, org, accounts)
	}
	if hasPipes == false {
		logrus.Errorf("Pipeline not found in config file")
		return "", err
	}
	newContent, _ := manifest.Encode()
	content = fmt.Sprintf("---\n%s", string(newContent))
	return content, nil
}

// Find will find the .strithon.yml config file in the GitHub repo and get it
func (p *Plugin) Find(ctx context.Context, req *config.Request) (*drone.Config, error) {
	logrus.Debug(ctx)
	logrus.Debug(req)
	// get the drone configuration file from the github repository
	content, err := p.GetGithubFile(ctx, req, req.Repo.Namespace, req.Repo.Name, req.Repo.Config)
	if err != nil {
		return nil, err
	}
	if content == "" {
		return nil, nil
	}

	// inject the api keys
	envs := []string{"qa", "pr"}
	var token string
	var key string
	for _, env := range envs {
		content, key, err = p.InjectKey(content, req, env)
		if env == os.Getenv("ENV") {
			token = key
		}
		if err != nil {
			logrus.Errorf("Error doing the injection for environment %s: %s", env, err)
			return nil, err
		}
	}

	// check permission for the repo to deploy to those accounts
	accts, err := p.Validate(ctx, req, token)
	if err != nil {
		return nil, err
	}
	logrus.Debugf("Result from validate: %v, err: %v", accts, err)
	if len(accts) > 0 {
		repo := req.Repo.Name
		org := req.Repo.Namespace
		var newPipe, err = p.replaceWarnings(content, repo, org, accts)
		if err != nil {
			return nil, err
		}
		// var pipeToString = string(pipe)
		return &drone.Config{
			Data: newPipe,
			Kind: "drone.v1.yaml",
		}, nil
	}

	return &drone.Config{
		Data: content,
		Kind: "drone.v1.yaml",
	}, nil
}
