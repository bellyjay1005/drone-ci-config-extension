package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/99designs/httpsignatures-go"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/drone/drone-go/plugin/config"
	"github.com/google/go-github/github"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

const (
	// headerSignature is the name of the header Signature
	headerSignature = "signature"

	// headerSiAuthorization is the name of the header Authorization
	headerAuthorization = "Authorization"

	// authScheme is the auth scheme wanted
	authScheme = "Signature "

	// entServer represents an Enterprise base URL of a Github server
	entServer = "https://github.com/api/v3/"

	// pubServer represents the public github base URL
	pubServer = "https://api.github.com/"
)

// client holds a github API client
var client *github.Client

type (
	// ClientAccessInfo holds the domain endpoint for an
	// Enterprise GitHub and the required access token.
	// This Defaults to the public GitHub API.
	clientAccessInfo struct {
		baseURL     string
		accessToken string
	}
)

// GetGithubServer returns Github Server endpoint from events
func GetGithubServer(evt events.APIGatewayProxyRequest) (string, error) {
	// define error variable
	var err error

	// declare event payload
	eventBody := evt.Body

	// Unmarshal the request payload into a value pointed at by the pointer
	droneHTTPRequest := &config.Request{}
	err = json.Unmarshal([]byte(eventBody), droneHTTPRequest)
	if err != nil {
		logrus.Debug("config: cannot unmarshal http.Request body")
		return "Invalid event payload from source", nil
	}

	// check and return source server domain endpoint
	if strings.Contains(droneHTTPRequest.Repo.Link, "https://github.com/") {
		return entServer, nil
	}
	return pubServer, nil
}

// CreateSSMService returns an AWS SSM session for a target region
func CreateSSMService(region string) (*ssm.SSM, error) {
	// create aws ssm session
	sess, _ := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	ssmClient := ssm.New(sess)
	return ssmClient, nil
}

// GetSSMParam returns an aws ssm-parameter token using region and parameter name
func GetSSMParam(region, tokenLocation string) (string, error) {
	var token string
	decryption := true

	// create aws ssm session
	ssmClient, err := CreateSSMService(region)

	// get token response
	tokenRes, err := ssmClient.GetParameter(&ssm.GetParameterInput{
		Name:           &tokenLocation,
		WithDecryption: &decryption,
	})
	if err != nil {
		return fmt.Sprintf("Error connecting to ssm: %s", err), nil
	}
	token = *(*tokenRes.Parameter).Value
	return token, nil
}

// CreateEnterpriseClient generates an Enterprise GitHub client.
func CreateEnterpriseClient(ctx context.Context, server, token string) (*github.Client, error) {
	// declare variables
	var err error

	// creates a github transport that authenticates
	// enterprise-github http requests using the github access token.
	trans := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	))

	// return an enterprise Github API client
	client, err = github.NewEnterpriseClient(server, server, trans)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// CreatePublicClient generates a Public GitHub client.
func CreatePublicClient(ctx context.Context, server, token string) (*github.Client, error) {
	// creates a github transport that authenticates
	// public-github http requests using the github access token.
	trans := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	))

	// return a new GitHub API client using above github transport
	// http.Client() to perform authentication
	client = github.NewClient(trans)
	return client, nil
}

// CreateClientAPI return appropriate Github API client based on target base-URL
func CreateClientAPI(server, region, entTokenLocation, pubTokenLocation string) (*github.Client, string, error) {
	// get tokens
	entGithubToken, _ := GetSSMParam(region, entTokenLocation)
	pubGithubToken, _ := GetSSMParam(region, pubTokenLocation)

	// create client
	if server == entServer {
		client, err := CreateEnterpriseClient(context.Background(), server, entGithubToken)
		logrus.Debugf("Server -  %v\n Token_Location - %v\n", server, entTokenLocation)
		if err != nil {
			logrus.Errorf("Error creating github enterprise client")
		}
		return client, entGithubToken, nil
	} else if server == pubServer {
		client, err := CreatePublicClient(context.Background(), server, pubGithubToken)
		logrus.Debugf("Server -  %v\n Token_Location - %v\n", server, pubTokenLocation)
		if err != nil {
			logrus.Errorf("Error creating public github client")
		}
		return client, pubGithubToken, nil
	}
	logrus.Errorf("Wrong Github server domain endpoint - %s", server)
	return client, entGithubToken, nil
}

// HTTPError converts a message and status code into a response that can be sent
func HTTPError(message string, code int) events.APIGatewayProxyResponse {
	return events.APIGatewayProxyResponse{
		Headers: map[string]string{
			"Content-Type":           "text/plain; charset=utf-8",
			"X-Content-Type-Options": "nosniff",
		},
		StatusCode: code,
		Body:       message,
	}
}

// ConstructHost construct strings of endpoint and host
func ConstructHost(env string) (string, string, error) {
	envInsert := ""
	if env == "qa" {
		envInsert = "-qa"
	}
	authEndpoint := fmt.Sprintf("https://demo-auth%s.strithon-cloud.com", envInsert)
	host := fmt.Sprintf("https://drone%s.strithon-cloud.com", envInsert)
	return authEndpoint, host, nil
}

// FromRequest mimics httpsignatures.FromRequest, but with a Request struct
func FromRequest(r events.APIGatewayProxyRequest) (*httpsignatures.Signature, error) {
	if s, ok := r.Headers[headerSignature]; ok {
		return httpsignatures.FromString(s)
	}
	if s, ok := r.MultiValueHeaders[headerSignature]; ok {
		return httpsignatures.FromString(s[0])
	}
	if a, ok := r.Headers[headerAuthorization]; ok {
		return httpsignatures.FromString(strings.TrimPrefix(a, authScheme))
	}
	if a, ok := r.MultiValueHeaders[headerAuthorization]; ok {
		return httpsignatures.FromString(strings.TrimPrefix(a[0], authScheme))
	}
	return nil, httpsignatures.ErrorNoSignatureHeader
}

// AddCapHeader adds capitalization to Header Keys
func AddCapHeader(req events.APIGatewayProxyRequest, header map[string][]string) (map[string][]string, error) {
	for h, val := range req.Headers {
		if !strings.HasPrefix(h, "x-") {
			// Add capitalization back in since http/2 removes header key capitalization
			var link = regexp.MustCompile("(^[A-Za-z])|(-[A-Za-z])")
			newH := link.ReplaceAllStringFunc(h, func(s string) string {
				return strings.ToUpper(strings.Replace(s, "_", "", -1))
			})

			header[newH] = []string{val}
		}
	}
	return header, nil
}

// GetRepoLink extracts Github repository link from string input
// by removing `https://` from each links
func GetRepoLink(HTTPLink string) string {
	return strings.ReplaceAll(HTTPLink, "https://", "")
}
