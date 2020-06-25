package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
)

// empty github client
var emptyClient = github.NewClient(nil)

// mockSSMClient mocks a struct to be used to mock AWS ssm client calls
type mockSSMClient struct {
	ssmiface.SSMAPI
	mockResponse map[string]ssm.GetParameterOutput
	err          bool
}

func (m mockSSMClient) GetParameter(in *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
	if val, ok := m.mockResponse[*in.Name]; ok {
		return &val, nil
	}
	return nil, errors.New("Error finding ssm parameter")
}
func TestGetGithubServer(t *testing.T) {
	// read json from file
	inputJSON, inErr := ioutil.ReadFile("testdata/request.json")
	if inErr != nil {
		t.Errorf("could not open test file. details: %v", inErr)
	}

	var inputEvent events.APIGatewayProxyRequest

	// de-serialize into Go object
	err := json.Unmarshal(inputJSON, &inputEvent)
	if err != nil {
		t.Errorf("could not unmarshal event payload. Details: %v", err)
	}
	fmt.Printf("RequestBody - %v\n", inputEvent.Body)

	// validate custom request events
	githubServer, _ := GetGithubServer(inputEvent)

	if githubServer != entServer {
		t.Errorf("Got unexpected github server base URL - %s", githubServer)
	}

	// assert payload is equal
	assert.JSONEq(
		t,
		string(inputEvent.Body),
		string(
			"{\"build\":{\"id\":1063,\"link\":\"https://github.com/bellyjay1005/aws-qa-drone-e2e-testing/compare/93e81eca70dd...1e81868cd723\"},\"repo\":{\"id\":1595,\"link\":\"https://github.com/bellyjay1005/aws-qa-drone-e2e-testing\"}}",
		),
	)

}

func TestGetSSMParam(t *testing.T) {
	// Setup Test
	token, err := GetSSMParam("us-east-1", "/qa/application-api/url")

	if err != nil {
		t.Errorf("Got unexpected ssm client failure - %s", err)
	}
	fmt.Printf("Token value: %s", token)

	// assert payload is equal
	assert.Equal(t, token, "https://jrv3xtesxfgpdnxuop4mfs54ba.appsync-api.us-east-1.amazonaws.com/graphql")
}

func TestCreateEnterpriseClient(t *testing.T) {
	token := "token"
	cases := []struct {
		url         string
		createError bool
	}{
		{
			url:         "https://github.com",
			createError: false,
		},
		{
			url:         "https://github.com",
			createError: false,
		},
		{
			url:         ":invalidurl",
			createError: true,
		},
	}
	for _, c := range cases {
		_, err := CreateEnterpriseClient(noContext, c.url, token)
		if (err == nil) == c.createError {
			t.Fatalf("Incorrect error %v, wanted %t", err, c.createError)
		}
	}
}

func TestCreatePublicClient(t *testing.T) {
	token := "token"
	cases := []struct {
		url         string
		createError bool
	}{
		{
			url: "https://github.com",
		},
		{
			url: "https://github.com",
		},
		{
			url: ":invalidurl",
		},
	}
	for _, c := range cases {
		client, _ := CreatePublicClient(noContext, c.url, token)
		assert.NotEqual(t, client, emptyClient, "NewClient returned same http.Clients, but they should differ")
	}
}

func TestCreateClientAPI(t *testing.T) {
	const (
		entTokenLocation = "/github/oauth_token"
		pubTokenLocation = "/github/pub/oauth_token"
		region           = "us-east-1"
	)
	cases := []struct {
		server string
	}{
		{
			server: "https://github.com/api/v3/",
		},
		{
			server: "https://api.github.com/",
		},
		{
			server: ":invalidurl",
		},
	}
	for _, c := range cases {
		_, _, err := CreateClientAPI(c.server, region, entTokenLocation, pubTokenLocation)
		if err != nil {
			t.Fatalf("Incorrect client %v", err)
		}
	}
}
