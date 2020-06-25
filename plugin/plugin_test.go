package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
	"github.com/drone/drone-go/drone"
	"github.com/drone/drone-go/plugin/config"
	"github.com/drone/drone-yaml/yaml"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"golang.org/x/oauth2"
)

// empty context
var noContext = context.Background()

// mock github token
const (
	mockToken = "d7c559e677ebc489d4e0193c8b97a12e"
)

type mockedSSM struct {
	ssmiface.SSMAPI
	respMap     map[string]ssm.GetParameterOutput
	respPathMap map[string]ssm.GetParametersByPathOutput
	err         bool
}

func (m mockedSSM) GetParameter(in *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
	if val, ok := m.respMap[*in.Name]; ok {
		return &val, nil
	}
	return nil, errors.New("Error finding parameter")
}

func (m mockedSSM) GetParametersByPath(in *ssm.GetParametersByPathInput) (*ssm.GetParametersByPathOutput, error) {
	if val, ok := m.respPathMap[*in.Path]; ok {
		return &val, nil
	}
	return nil, errors.New("Error finding parameter")
}

func TestPayloadGenerator(t *testing.T) {
	id := "one"
	secret := "two"
	audience := "three"
	user := "four"
	link := "github.com/bellyjay1005/aws-drone-policy"
	want := `{"client_id":"one","client_secret":"two","audience":"three","grant_type":"client_credentials","drone_username":"four","state":"{\"namespace\":\"demo\",\"data\":[[\"repository\",\"github.com/bellyjay1005/aws-drone-policy\"],[\"sender\",\"four\"]]}"}`

	payload, _ := GeneratePayload(id, secret, audience, user, link)
	if payload != want {
		t.Errorf("Expected %s got %s", want, payload)
	}
}

func TestGetAuth0APIKey(t *testing.T) {
	token := "token"
	tokenResp := fmt.Sprintf(`{"access_token":"%s","scope":"name","expires_in":86400,"token_type":"Bearer"}`, token)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if strings.HasPrefix(r.URL.EscapedPath(), "/api/repos/valid") {
			t.Errorf("Blob")
		}
		w.Write([]byte(tokenResp))
	}))
	defer ts.Close()

	env := "pr"
	clientKey := fmt.Sprintf("/%s/auth0/client-id", env)
	secretKey := fmt.Sprintf("/%s/auth0/client-secret", env)
	cases := []struct {
		fakeSSM mockedSSM
		want    string
		err     bool
		url     string
	}{
		{
			fakeSSM: mockedSSM{
				respPathMap: map[string]ssm.GetParametersByPathOutput{
					fmt.Sprintf("/%s/auth0/", env): ssm.GetParametersByPathOutput{
						Parameters: []*ssm.Parameter{
							&ssm.Parameter{
								Name:  &clientKey,
								Value: &clientKey,
							},
							&ssm.Parameter{
								Name:  &secretKey,
								Value: &secretKey,
							},
						},
					},
				},
				err: false,
			},
			want: token,
			url:  ts.URL,
		},
		{
			fakeSSM: mockedSSM{
				respPathMap: map[string]ssm.GetParametersByPathOutput{
					fmt.Sprintf("/%s/auth0/", env): ssm.GetParametersByPathOutput{
						Parameters: []*ssm.Parameter{
							&ssm.Parameter{
								Name:  &secretKey,
								Value: &secretKey,
							},
						},
					},
				},
				err: false,
			},
			want: "",
			err:  true,
			url:  ts.URL,
		},
		{
			fakeSSM: mockedSSM{
				respPathMap: map[string]ssm.GetParametersByPathOutput{
					fmt.Sprintf("/%s/auth0/", env): ssm.GetParametersByPathOutput{
						Parameters: []*ssm.Parameter{
							&ssm.Parameter{
								Name:  &clientKey,
								Value: &clientKey,
							},
						},
					},
				},
				err: false,
			},
			want: "",
			err:  true,
			url:  ts.URL,
		},
	}

	for _, c := range cases {
		p := New("", "", "", "", ts.URL, "", c.url, c.url+"hi", c.fakeSSM, nil)
		req := config.Request{
			Build: drone.Build{
				Sender: "name",
			},
		}
		got, err := p.GetAuth0APIKey(&req, env)
		if got != c.want {
			t.Errorf("got api key %s, wanted %s", got, c.want)
		}
		if (err == nil) == c.err {
			t.Errorf("expected receiving error %v, but got err %v", c.err, err)
		}
	}
}

func TestEncryptData(t *testing.T) {
	enc := "YXNkZgo="
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.EscapedPath(), "/api/repos/valid") {
			w.Write([]byte(fmt.Sprintf(`{"Data": "%v"}`, enc)))
		}
		if strings.HasPrefix(r.URL.EscapedPath(), "/api/repos/invalid") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"NoData": "%v"}`))
			return
		}
	}))
	defer ts.Close()
	cases := []struct {
		namespace string
		want      string
	}{
		{
			namespace: "valid",
			want:      enc,
		},
		{
			namespace: "invalid",
			want:      "",
		},
	}
	p := New(ts.URL, "", "faketoken", ts.URL, "", "", "", "", nil, nil)
	for _, r := range cases {
		req := config.Request{
			Repo: drone.Repo{
				Namespace: r.namespace,
				Name:      "name",
			},
		}
		got, _ := p.EncryptData("content", &req)
		if got != r.want {
			t.Errorf("got %s, but expected %s", got, r.want)
		}
	}
}

func TestInjectKey(t *testing.T) {
	encSecret := "YXNkZgo="
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.EscapedPath(), "/api/repos/valid") {
			w.Write([]byte(fmt.Sprintf(`{"Data": "%v"}`, encSecret)))
		}
		if strings.HasPrefix(r.URL.EscapedPath(), "/api/repos/invalid") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"NoData": "v"}`))
		}
		if strings.HasPrefix(r.URL.EscapedPath(), "/api/repos/yml") {
			w.Write([]byte(fmt.Sprintf(`{"Data": "%v"}`, encSecret)))
		} else {
			w.Write([]byte("data"))
		}
	}))
	defer ts.Close()
	content, _ := ioutil.ReadFile("testdata/.drone.yml")
	contentMulti, _ := ioutil.ReadFile("testdata/.drone-multi.yml")

	validReplace, _ := ioutil.ReadFile("testdata/.drone-inserted.yml")
	qaReplace, _ := ioutil.ReadFile("testdata/.drone-inserted-qa.yml")
	invalidYml, _ := ioutil.ReadFile("testdata/.drone-invalid.yml")
	noPipeYml, _ := ioutil.ReadFile("testdata/.drone-nopipeline.yml")
	multiPipeYml, _ := ioutil.ReadFile("testdata/.drone-multi-pipeline.yml")
	// Environment is left as "" since only the suffix matters for this method
	clientKey := "//auth0/client-id"
	secretKey := "//auth0/client-secret"
	validMap := map[string]ssm.GetParametersByPathOutput{
		fmt.Sprintf("/%s/auth0/", "pr"): ssm.GetParametersByPathOutput{
			Parameters: []*ssm.Parameter{
				&ssm.Parameter{
					Name:  &clientKey,
					Value: &clientKey,
				},
				&ssm.Parameter{
					Name:  &secretKey,
					Value: &secretKey,
				},
			},
		},
		fmt.Sprintf("/%s/auth0/", "qa"): ssm.GetParametersByPathOutput{
			Parameters: []*ssm.Parameter{
				&ssm.Parameter{
					Name:  &clientKey,
					Value: &clientKey,
				},
				&ssm.Parameter{
					Name:  &secretKey,
					Value: &secretKey,
				},
			},
		},
	}
	cases := []struct {
		content   string
		namespace string
		want      string
		wantToken string
		ssm       mockedSSM
		err       bool
		env       string
	}{
		{
			content:   string(contentMulti),
			namespace: "valid",
			want:      string(multiPipeYml),
			ssm: mockedSSM{
				respPathMap: validMap,
				err:         false,
			},
			err: false,
			env: "pr",
		},
		{
			content:   string(content),
			namespace: "valid",
			want:      string(validReplace),
			ssm: mockedSSM{
				respPathMap: validMap,
				err:         false,
			},
			err: false,
			env: "pr",
		},
		{
			content:   string(content),
			namespace: "validqa",
			want:      string(qaReplace),
			ssm: mockedSSM{
				respPathMap: validMap,
				err:         false,
			},
			err: false,
			env: "qa",
		},
		{
			content:   string(content),
			namespace: "invalid",
			want:      string(content),
			ssm: mockedSSM{
				respPathMap: validMap,
				err:         false,
			},
			err: true,
			env: "pr",
		},
		{
			content:   string(invalidYml),
			namespace: "ymlInvalid",
			want:      "",
			ssm: mockedSSM{
				respPathMap: validMap,
				err:         false,
			},
			err: true,
			env: "pr",
		},
		{
			content:   string(noPipeYml),
			namespace: "ymlNoPipeline",
			want:      "",
			ssm: mockedSSM{
				respPathMap: validMap,
				err:         false,
			},
			err: true,
			env: "pr",
		},
		{
			content:   string(noPipeYml),
			namespace: "ymlNoPipeline2",
			want:      "",
			ssm: mockedSSM{
				respPathMap: map[string]ssm.GetParametersByPathOutput{
					fmt.Sprintf("/%s/auth0/", "pr"): ssm.GetParametersByPathOutput{
						Parameters: []*ssm.Parameter{},
					},
				},
				err: false,
			},
			err: true,
			env: "pr",
		},
	}
	for _, c := range cases {
		p := New(ts.URL, "", "faketoken", ts.URL, ts.URL, "", ts.URL, ts.URL, c.ssm, nil)
		req := config.Request{
			Repo: drone.Repo{
				Namespace: c.namespace,
				Name:      "name",
			},
		}
		// TODO: qa testing
		got, token, err := p.InjectKey(c.content, &req, c.env)
		if got != string(c.want) {
			t.Errorf("%s: got %s, but expected %s", c.namespace, got, c.want)
		}
		if token != string(c.wantToken) {
			t.Errorf("%s: got token %s, but expected %s", c.namespace, token, c.wantToken)
		}
		if (err == nil) == c.err {
			t.Errorf("%s: Expect receiving error to be %v, but got error %v", c.namespace, c.err, err)
		}
	}
}

func TestGetGithubFile(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.EscapedPath(), "/repos/error") {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("File not found at url: " + r.URL.EscapedPath()))
		}
		if strings.HasPrefix(r.URL.EscapedPath(), "/repos/nil") {
			out, _ := ioutil.ReadFile("testdata/nil-contents.json")
			w.Write(out)
		}
		if strings.HasPrefix(r.URL.EscapedPath(), "/repos/invalid") {
			out, _ := ioutil.ReadFile("testdata/invalid-contents.json")
			w.Write(out)
		}
		out, _ := ioutil.ReadFile("testdata/contents.json")
		w.Write(out)
	}))
	defer ts.Close()

	trans := oauth2.NewClient(noContext, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: mockToken},
	))

	client, err := github.NewEnterpriseClient(ts.URL, ts.URL, trans)
	if err != nil {
		t.Errorf("Error creating enterprise client for mock URL: %v", err)
	}
	p := New(ts.URL, mockToken, "", ts.URL, "", "", "", "", nil, client)
	req := &config.Request{
		Build: drone.Build{
			After: "a1afc9b699274831f841d1fd8ace0f5e91d92711",
		},
		Repo: drone.Repo{
			Slug:   "octocat/hello-world",
			Config: ".drone.yml",
		},
	}

	got, err := p.GetGithubFile(noContext, req, req.Repo.Namespace, req.Repo.Name, ".strithon.yml")
	if err != nil {
		t.Errorf("Error getting GitHub file: %v", err)
	}

	got, err = p.GetGithubFile(noContext, req, "error"+req.Repo.Namespace, req.Repo.Name, ".strithon.yml")
	if err == nil {
		t.Errorf("Expected error getting file, instead got %v", err)
	}

	got, err = p.GetGithubFile(noContext, req, "nil"+req.Repo.Namespace, req.Repo.Name, ".strithon.yml")
	if err != nil {
		t.Errorf("Expected no error getting file, instead got %v", err)
	}
	if got != "" {
		t.Errorf("Expected \"\", but got \"%s\"", got)
	}

	got, err = p.GetGithubFile(noContext, req, "invalid"+req.Repo.Namespace, req.Repo.Name, ".strithon.yml")
	if err == nil {
		t.Errorf("Expected error getting file, instead got %v", err)
	}
}

func TestValidate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.EscapedPath(), "/repos/error") {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("File not found at url: " + r.URL.EscapedPath()))
		} else if strings.HasPrefix(r.URL.EscapedPath(), "/repos/nil") {
			out, _ := ioutil.ReadFile("testdata/nil-contents.json")
			w.Write(out)
		} else if strings.HasPrefix(r.URL.EscapedPath(), "/repos/invalid") {
			out, _ := ioutil.ReadFile("testdata/invalid-yml-contents.json")
			w.Write(out)
		} else if strings.HasPrefix(r.URL.EscapedPath(), "/repos/no-environ") {
			out, _ := ioutil.ReadFile("testdata/contents-no-environ.json")
			w.Write(out)
		} else if strings.HasPrefix(r.URL.EscapedPath(), "/v1") {
			b, _ := ioutil.ReadAll(r.Body)
			var body AuthRequest
			err := json.Unmarshal(b, &body)
			if err != nil {
				t.Fatalf("Error unmarshalling request body")
			}
			result := true
			if body.Input.Repo == "noaccess" {
				result = false
			}
			resp := AuthResponse{
				Result:     result,
				DecisionID: "506a05fc-8354-415b-b6b5-e32e8af60255",
			}
			out, err := json.Marshal(resp)
			w.Write(out)
		} else {
			out, _ := ioutil.ReadFile("testdata/contents.json")
			w.Write(out)
		}
	}))
	defer ts.Close()

	trans := oauth2.NewClient(noContext, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: mockToken},
	))

	client, err := github.NewEnterpriseClient(ts.URL, ts.URL, trans)
	if err != nil {
		t.Errorf("Error creating enterprise client for mock URL: %v", err)
	}

	validMap := map[string]ssm.GetParameterOutput{}

	cases := []struct {
		name      string
		namespace string
		err       bool
		valid     bool
		ssm       mockedSSM
	}{
		{
			name:      "happypath",
			namespace: "happypath",
			err:       false,
			valid:     true,
			ssm: mockedSSM{
				respMap: validMap,
				err:     false,
			},
		},
		{
			name:      "no-environ",
			namespace: "no-environ",
			err:       false,
			valid:     true,
			ssm: mockedSSM{
				respMap: validMap,
				err:     false,
			},
		},
		{
			name:      "no access to listed account",
			namespace: "noaccess",
			err:       false,
			valid:     true,
			ssm: mockedSSM{
				respMap: validMap,
				err:     false,
			},
		},
		{
			name:      "no strithon.yml contents",
			namespace: "nil",
			err:       false,
			valid:     false,
		},
		{
			name:      "error connecting to github",
			namespace: "error",
			err:       true,
			valid:     false,
		},
		{
			name:      "invalid yaml file",
			namespace: "invalid",
			err:       true,
			valid:     false,
		},
	}

	for _, c := range cases {
		req := &config.Request{
			Build: drone.Build{
				After: "a1afc9b699274831f841d1fd8ace0f5e91d92711",
			},
			Repo: drone.Repo{
				Namespace: c.namespace,
				Slug:      "octocat/hello-world",
				Config:    ".drone.yml",
			},
		}

		p := New(ts.URL, mockToken, "", ts.URL, ts.URL, ts.URL, "", "", c.ssm, client)
		invalidAccts, err := p.Validate(noContext, req, "")

		if c.err == (err == nil) {
			t.Errorf("%v Expected receiving error to be %v, but got %v", c.name, c.err, err)
		}
		if (c.valid) != (len(invalidAccts) < 1 && invalidAccts != nil) {
			t.Errorf("%v Expected file validity to be %v, but received %v", c.name, c.valid, invalidAccts)
		}
	}
}

func TestInjectWarnings(t *testing.T) {
	injectYamlFile, _ := ioutil.ReadFile("testdata/.drone-plugins.yml")
	var injectYamlContent = string(injectYamlFile)
	manifest, err := yaml.Parse(strings.NewReader(injectYamlContent))
	if err != nil {
		return
	}
	var repo = "unicorn_plugin"
	var org = "bellyjay1005"
	var accounts = []string{"fake_qa", "fake_prod"}
	for _, r := range manifest.Resources {
		v, ok := r.(*yaml.Pipeline)
		if !ok {
			continue
		}
		injectWarnings(v, repo, org, accounts)
	}
	newContent, _ := manifest.Encode()
	var got = fmt.Sprintf("---\n%s", string(newContent))
	var want = "Update permissions in https://github.com/bellyjay1005/aws-drone-policy."
	assert.Contains(t, got, want, "error message %s", "formatted")
}

func TestReplaceWarnings(t *testing.T) {
	yamlFile, _ := ioutil.ReadFile("testdata/.drone-plugins.yml")
	var yamlContent = string(yamlFile)
	var repo = "bigfoot_plugin"
	var org = "bellyjay1005"
	var accounts = []string{"fake_qa", "fake_prod"}
	p := New("", "", "", "", "", "", "", "", nil, nil)
	var got, _ = p.replaceWarnings(yamlContent, repo, org, accounts)
	var want = "Update permissions in https://github.com/bellyjay1005/aws-drone-policy."
	print(got)
	assert.Containsf(t, got, want, "error message %s", "formatted")
}

func TestFind(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.EscapedPath(), "/repos/error") {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("File not found at url: " + r.URL.EscapedPath()))
		} else if strings.HasPrefix(r.URL.EscapedPath(), "/repos/nil") {
			out, _ := ioutil.ReadFile("testdata/nil-contents.json")
			w.Write(out)
		} else if strings.HasPrefix(r.URL.EscapedPath(), "/api/repos") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"access_token": "foo", "refresh_token": "bar"}`))
		} else if strings.HasPrefix(r.URL.EscapedPath(), "/v1") {
			// TODO: update this
			b, _ := ioutil.ReadAll(r.Body)
			var body AuthRequest
			err := json.Unmarshal(b, &body)
			if err != nil {
				t.Fatalf("did a thing wrong")
			}
			resp := AuthResponse{
				Result:     true,
				DecisionID: "506a05fc-8354-415b-b6b5-e32e8af60255",
			}
			out, err := json.Marshal(resp)
			if strings.HasPrefix(r.URL.EscapedPath(), "/v1") {
				w.Write(out)
			}
		} else {
			out, _ := ioutil.ReadFile("testdata/contents.json")
			w.Write(out)
		}
	}))
	defer ts.Close()

	trans := oauth2.NewClient(noContext, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: mockToken},
	))

	client, err := github.NewEnterpriseClient(ts.URL, ts.URL, trans)
	if err != nil {
		t.Errorf("Error creating enterprise client for mock URL: %v", err)
	}

	validMap := map[string]ssm.GetParameterOutput{}

	env := "pr"
	clientKey := fmt.Sprintf("/%s/auth0/client-id", env)
	secretKey := fmt.Sprintf("/%s/auth0/client-secret", env)
	validPathMap := map[string]ssm.GetParametersByPathOutput{
		fmt.Sprintf("/%s/auth0/", env): ssm.GetParametersByPathOutput{
			Parameters: []*ssm.Parameter{
				&ssm.Parameter{
					Name:  &clientKey,
					Value: &clientKey,
				},
				&ssm.Parameter{
					Name:  &secretKey,
					Value: &secretKey,
				},
			},
		},
		fmt.Sprintf("/%s/auth0/", "qa"): ssm.GetParametersByPathOutput{
			Parameters: []*ssm.Parameter{
				&ssm.Parameter{
					Name:  &clientKey,
					Value: &clientKey,
				},
				&ssm.Parameter{
					Name:  &secretKey,
					Value: &secretKey,
				},
			},
		},
	}

	cases := []struct {
		namespace string
		err       bool
		content   bool
		ssm       mockedSSM
	}{
		{
			namespace: "happypath",
			err:       false,
			content:   true,
			ssm: mockedSSM{
				respMap:     validMap,
				respPathMap: validPathMap,
				err:         false,
			},
		},
		{
			namespace: "happypath-n",
			err:       false,
			content:   true,
			ssm: mockedSSM{
				respMap:     validMap,
				respPathMap: validPathMap,
				err:         false,
			},
		},
		{
			namespace: "nil",
			err:       false,
			content:   false,
			ssm: mockedSSM{
				respMap:     validMap,
				respPathMap: validPathMap,
				err:         false,
			},
		},
		{
			namespace: "error",
			err:       true,
			content:   false,
			ssm: mockedSSM{
				respMap:     validMap,
				respPathMap: validPathMap,
				err:         false,
			},
		},
		{
			namespace: "noaccess",
			err:       false,
			content:   true,
			ssm: mockedSSM{
				respMap:     validMap,
				respPathMap: validPathMap,
				err:         false,
			},
		},
		{
			namespace: "ssmerror",
			err:       true,
			ssm: mockedSSM{
				respMap: map[string]ssm.GetParameterOutput{},
				err:     true,
			},
		},
		{
			namespace: "noparams",
			err:       true,
			content:   true,
			ssm: mockedSSM{
				respMap: validMap,
				respPathMap: map[string]ssm.GetParametersByPathOutput{
					fmt.Sprintf("/%s/auth0/", env): ssm.GetParametersByPathOutput{
						Parameters: []*ssm.Parameter{},
					},
				},
				err: false,
			},
		},
	}

	for _, c := range cases {
		req := &config.Request{
			Build: drone.Build{
				After: "a1afc9b699274831f841d1fd8ace0f5e91d92711",
			},
			Repo: drone.Repo{
				Namespace: c.namespace,
				Slug:      "octocat/hello-world",
				Config:    ".drone.yml",
			},
		}
		p := New(ts.URL, mockToken, "faketoken", ts.URL, ts.URL, ts.URL, "", "", c.ssm, client)
		config, err := p.Find(noContext, req)

		if c.err == (err == nil) {
			t.Errorf("%s: Expected receiving error to be %v, but got %v", c.namespace, c.err, err)
		}
		if c.content && (config != nil) && (config.Kind != "drone.v1.yaml") {
			t.Errorf("%s: Expected kind: service, instead got %s", c.namespace, config.Kind)
		}
		if !c.content && config != nil {
			t.Errorf("%s: Expected no content, but received %v", c.namespace, config)
		}
	}
}
