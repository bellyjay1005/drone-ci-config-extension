package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/drone/drone-go/plugin/config"
	"github.com/sirupsen/logrus"
	"github.com/bellyjay1005/aws-config-check-extension/plugin"
)

const (
	// auth0Endpoint represents the Auth0 endpoint URL
	auth0Endpoint = "https://bellyjay1005-id.auth0.com/oauth/token"
)

// HandleRequest handles the input from lambda
func HandleRequest(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// initialize variables
	logrus.Debugf("Initializing function variables")
	var tokenLocation string
	body := req.Body
	droneReq := &config.Request{}
	combinedHeaders := make(map[string][]string)
	env := os.Getenv("ENV")
	region := os.Getenv("REGION")
	entTokenLocation := os.Getenv("GITHUB_ENT_TOKEN_LOC")
	pubTokenLocation := os.Getenv("GITHUB_PUB_TOKEN_LOC")
	ssmClient, _ := plugin.CreateSSMService(region)
	tokenLocation = fmt.Sprintf("/%s/drone-demo/drone_token", env)
	secretLocation := fmt.Sprintf("/%v/drone-demo/config_ext_secret", env)

	// get log-level for production
	if os.Getenv("LOGLEVEL") == "DEBUG" {
		logrus.SetLevel(logrus.DebugLevel)
	}
	logrus.Debugf("Context %v\nRequest%v\n", ctx, req)

	// check target github server base URL
	server, err := plugin.GetGithubServer(req)
	if err != nil {
		logrus.Errorf("Wrong Github server domain endpoint. Check your Drone CI configurations - %s", err)
	}

	// return github API client and token
	client, token, _ := plugin.CreateClientAPI(server, region, entTokenLocation, pubTokenLocation)

	// get demo token
	droneToken, err := plugin.GetSSMParam(region, tokenLocation)
	if err != nil {
		return plugin.HTTPError(fmt.Sprintf("Error connecting to ssm: %s", err), 500), nil
	}

	// get auth-endpoint and host name
	authEndpoint, host, _ := plugin.ConstructHost(env)

	// declare plugin method
	p := plugin.New(
		server,
		token,
		droneToken,
		host,
		auth0Endpoint,
		authEndpoint,
		os.Getenv("API_ENDPOINT"),
		os.Getenv("API_ENDPOINT_QA"),
		ssmClient,
		client,
	)

	// HTTP handling stuff from drone-go/handler.go
	signature, errorValue := plugin.FromRequest(req)
	if errorValue != nil {
		logrus.Error("config: invalid or missing signature in Request. Error - ", errorValue)
	}

	// re-add header capitalization
	combinedHeaders, _ = plugin.AddCapHeader(req, combinedHeaders)
	r := http.Request{
		Header: http.Header(combinedHeaders),
	}

	logrus.Debugf("Secret location: %v", secretLocation)
	token, err = plugin.GetSSMParam(region, secretLocation)
	if err != nil {
		return plugin.HTTPError("Error connecting to ssm", 500), nil
	}
	// handle validation of signature
	if !signature.IsValid(token, &r) {
		logrus.Debug("config: invalid signature in http.Request")
		return plugin.HTTPError("Invalid Signature", 400), nil
	}

	err = json.Unmarshal([]byte(body), droneReq)
	if err != nil {
		logrus.Debug("config: cannot unmarshal http.Request body")
		return plugin.HTTPError("Invalid Input", 400), nil
	}

	// get and validate .strithon.yml config file
	res, err := p.Find(r.Context(), droneReq)
	if err != nil {
		logrus.Debugf("config: cannot find configuration: %s: %s: %s",
			droneReq.Repo.Slug,
			droneReq.Build.Target,
			err,
		)
		return plugin.HTTPError(err.Error(), 404), nil
	}
	if res == nil {
		return events.APIGatewayProxyResponse{StatusCode: 204}, nil
	}
	out, _ := json.Marshal(res)

	return events.APIGatewayProxyResponse{
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body:       string(out),
		StatusCode: 200,
	}, nil
}

func main() {
	lambda.Start(HandleRequest)
}
