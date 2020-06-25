
'''
Helper scripts
'''
import os
import time
import logging
import boto3
import requests
from github import Github


# Initialize LOGGER
LOGGER = logging.getLogger()
LOGGER.setLevel(os.getenv('LOG_LEVEL', 'INFO'))

class ManageAWSResources():
    '''
    Get boto3 resources
    '''
    def __init__(self):
        '''
        Declare function initialization variable
        '''
        self.region = os.environ['AWS_DEFAULT_REGION']
        self.cloudformation = boto3.client('cloudformation', region_name=self.region)
        self.ssm = boto3.client('ssm', region_name=self.region)

    def get_stack_status(self, stack_name):
        '''
        Get deployed stack status from stack object with cloudformation plugin
        '''
        status = self.cloudformation.describe_stacks(StackName=stack_name)
        return status['Stacks'][0]['StackStatus']

    def get_database_host(self, stack_name):
        '''
        Get RDS database host name from cloudformation stack
        '''
        output = self.cloudformation.describe_stacks(StackName=stack_name)
        return output['Stacks'][0]['Outputs'][0]['OutputValue']

    def get_secret(self, secret_name):
        '''
        Get secret from ssm
        '''
        secret = self.ssm.get_parameter(Name=secret_name, WithDecryption=True)
        return secret['Parameter']['Value']

class ManageGithubAPI():
    '''
    Manage Github API
    '''
    def __init__(self, repo_name, target_branch):
        '''
        Initialize Github variable
        '''
        self.repo_name = repo_name
        self.branch = target_branch
        self.region = os.environ['AWS_DEFAULT_REGION']
        self.ssm = boto3.client('ssm', region_name=self.region)
        self.github_token = self.get_auth_token('/github/oauth_token')
        self.github = Github(
            base_url='https://github.com/api/v3',
            login_or_token=self.github_token
        )
        self.repo = self.github.get_repo(self.repo_name)

    def get_auth_token(self, secret_name):
        '''
        Get Auth token from ssm parameter store
        Args:
            secret_name: name of a ssm parameter
        Returns:
            secret: ssm parameter value
        '''
        LOGGER.info('Getting authorization token from ssm parameter store')
        secret = self.ssm.get_parameter(Name=secret_name, WithDecryption=True)
        return secret['Parameter']['Value']

    def create_new_github_file(self, file, message, content):
        '''
        Create a new file in a repository
        '''
        LOGGER.info('Creating a new file within repository - %s', self.repo_name)
        resp = self.repo.create_file(
            file,
            message,
            content,
            branch=self.branch
        )
        commit_sha = resp['commit']
        return commit_sha

    def update_github_file(self, file, ref, message, content):
        '''
        Update a file in a repository
        '''
        LOGGER.info('Updating a file within repository - %s', self.repo_name)
        contents = self.repo.get_contents(file, ref=ref)
        resp = self.repo.update_file(
            contents.path,
            message,
            content,
            contents.sha,
            branch=self.branch
        )
        commit_sha = resp['commit']

        # wait for plugin to create stacks
        time.sleep(10)
        return commit_sha

    def delete_github_file(self, file, ref, commit_comments):
        '''
        Delete a file in a repository
        '''
        LOGGER.info('Deleting a file within repository - %s', self.repo_name)
        contents = self.repo.get_contents(file, ref=ref)
        resp = self.repo.delete_file(
            contents.path,
            commit_comments,
            contents.sha,
            branch=self.branch
        )
        commit_sha = resp['commit']
        return commit_sha

class ManageDroneAPI():
    '''
    Manage Drone CI API
    '''
    def __init__(self, host, token_name, repo):
        '''
        Declare API variable
        '''
        self.host = host
        self.repository = repo
        self.region = os.environ['AWS_DEFAULT_REGION']
        self.ssm = boto3.client('ssm', region_name=self.region)
        self.token = self.get_secret(token_name)
        self.header = {"Authorization": f'token {self.token}'}

    def get_secret(self, secret_name):
        '''
        Get secret from ssm
        '''
        secret = self.ssm.get_parameter(Name=secret_name, WithDecryption=True)
        return secret['Parameter']['Value']

    def get_build_numbers(self):
        '''
        Get Drone build id from list of builds
        '''
        build_ids = []
        LOGGER.info('Getting list of deployed build ids')
        builds_url = f'{self.host}/api/repos/{self.repository}/builds'
        response = requests.get(builds_url, headers=self.header)
        json_response = response.json()
        for build in json_response:
            build_ids.append(build['number'])
        LOGGER.info('Got build id = %s', build_ids)
        return build_ids

    def get_build_response_status(self, build_number):
        '''
        Get API response status
        '''
        LOGGER.info('Getting build info for repository - %s', self.repository)
        get_url = f'{self.host}/api/repos/{self.repository}/builds/{build_number[0]}'
        response = requests.get(get_url, headers=self.header)
        return response.status_code

    def get_build_content(self, build_number):
        '''
        Get Drone build API response
        '''
        LOGGER.info('Getting build event contents for - %s', self.repository)
        content_url = f'{self.host}/api/repos/{self.repository}/builds/{build_number[0]}'
        response = requests.get(content_url, headers=self.header)
        return response.json()

    def get_drone_build_status(self, build_number):
        '''
        Get build info for a single build
        '''
        LOGGER.info('Getting build event info for build id =  %s', build_number[0])
        build_url = f'{self.host}/api/repos/{self.repository}/builds/{build_number[0]}'
        response = requests.get(build_url, headers=self.header)
        json_response = response.json()
        return json_response['status']

    def stop_drone_build_job(self, build_number):
        '''
        Stop specific Drone job
        '''
        LOGGER.info('Stopping target build job for build id =  %s', build_number[0])
        delete_url = f'{self.host}/api/repos/{self.repository}/builds/{build_number[0]}'
        requests.delete(delete_url, headers=self.header)
        return True

    def restart_drone_build_job(self, build_number):
        '''
        Restart specific Drone job
        '''
        LOGGER.info('Restarting target build job for build id =  %s', build_number[0])
        restart_url = f'{self.host}/api/repos/{self.repository}/builds/{build_number[0]}'
        response = requests.post(restart_url, headers=self.header)
        json_response = response.json()
        return json_response