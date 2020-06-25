# drone-ci-config-check-extension

Drone extension to check config permissions and perform custom transforms on drone templates.

## strithon file validation

Before creating a Drone job, this extension will pull the `strithon.yml` file for the job's repository. The sender - typically the committer - of the request will have their accesible accounts, pulled from ldap, [matched](https://github.com/bellyjay1005/aws-ldap-account-map) against those in the `environments` of the `strithon.yml` file. If the sender does not have access to one or more accounts listed, the drone job will be replaced with a single step called `Authentication`, which will throw an error and display a message with the accounts not accessible.

## API Key Injection

This extension will inject two auth0 tokens as encrypted environment variables into each step, `DEMO_API_TOKEN` and `DEMO_API_TOKEN_QA`. These can be treated as plaintext environment variables for authentication, but will not be echoed into the build logs.

## Notes

In order to put auth0 tokens into templates, the account that this is deployed into must have two ssm parameters created - `/{environment}/auth0/client-id` and `/{environment}/auth0/client-secret`, populated with the relevant id and secret.

Auth0 won't automatically include the `drone_username` specified in [plugin.go](plugin/plugin.go) in the JWT bearer token, so a hook must be added manually to Auth0 so that receivers of the token can differentiate users for access control.

To deploy this tool, ensure your target AWS account have ssm parameters created for - `/github/oauth_token` and `/github/pub/oauth_token`. These would be referenced within the [stack template](https://github.com/bellyjay1005/aws-config-check-extension/blob/master/templates/resource.yml) as environment variables. These values provide the necessary token's locations needed for both the Enterprise and Public Github servers respectively. The `config-check-extension` uses these values to authenticate each Github API clients to help with strithon-file extraction and validation.

For any other use case, use the state variable in the request to get a token:

`state = '{"namespace":"demo", "data": [["repository", "bellyjay1005/demo-policy-api"]]}'`

```js
/**
@param {object} client - information about the client
@param {string} client.name - name of client
@param {string} client.id - client id
@param {string} client.tenant - Auth0 tenant name
@param {object} client.metadata - client metadata
@param {array|undefined} scope - array of strings representing the scope claim or undefined
@param {string} audience - token's audience claim
@param {object} context - additional authorization context
@param {object} context.webtask - webtask context
@param {function} cb - function (error, accessTokenClaims)
*/
module.exports = function(client, scope, audience, context, cb) {
  var access_token = {};
  access_token.scope = scope;
  if ('drone_username' in context.body) {
    access_token.scope.push(context.body.drone_username);
  }
  if (context.body.state) {
    try{
      state = JSON.parse(context.body.state);
    } catch(e) {}
    if (state && state.data && state.namespace && Array.isArray(state.data)) {
      state.data.forEach((data) => {
        access_token[`https://${state.namespace}.strithon-cloud.com/${data[0]}`] = data[1];
      })
    }
  }

  cb(null, access_token);
};
```

## Initial testing framework setup (only do once)

Run `make gitea`. This will create the compose stack with mysql, gitea, drone, and the drone extension. On the first run, gitea and mysql will initialize. Set it up following these steps:

- Got to <http://localhost:3000/install> and hit the button at the bottom (don't make any changes to this page). This will redirect to an invalid URL. If it doesn't work right away wait for a minute before trying again
- Go to <http://localhost:3000> and hit register in the top right hand corner
- Make the user `testuser` with password `password`, with any email
- Once signed into that user, create a repo called `test-example`, but don't initialize it with any files
- Open the `test-example` folder in the aws-config-check-extension repo and run `make setup` to get files into your gitea repo

## Links

- Drone
  - URL: [http://localhost:8888](http://localhost:8888)
  - Username: testuser
  - Password: password
- Gitea
  - URL: [http://localhost:3000](http://localhost:3000)
  - Username: testuser
  - Password: password

## Testing webhooks

- Go to <http://localhost>, sign in with `testuser` and `password`
- Activate the `test-example` repo
- Go to <http://localhost:3000/testuser/test-example/settings/hooks>
- Select the hook, and click `Test Delivery`
- Go back to <http://localhost/testuser/test-example/> and see the build running
