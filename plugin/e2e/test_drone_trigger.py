'''
Test Drone CI builds trigger and status
using a synced sample Github repository
'''
import time
from tests.e2e import helper

REPO = 'bellyjay1005/aws-qa-drone-e2e-testing'
BRANCH = 'master'
DRONE_TARGET_HOST = 'https://drone-qa.strithon-cloud.com'
DRONE_API_TOKEN_NAME = '/qa/drone/e2e/application/token'
TARGET_REPO = 'bellyjay1005/aws-qa-drone-e2e-testing'
GITHUB = helper.ManageGithubAPI(REPO, BRANCH)
DRONE = helper.ManageDroneAPI(
        DRONE_TARGET_HOST,
        DRONE_API_TOKEN_NAME,
        TARGET_REPO
    )


def test_plugin_with_strithon_file():
    '''
    Trigger Drone build job with strithon-file to create stacks using plugin
    '''
    target_file = '.strithon.yml'
    ref = 'refs/heads/master'
    message = 'updated strithon file to trigger drone build job'
    content = '''---
kind: service
metadata:
  service:
    id: e9337d78-d3d8-11e9-bb65-2a2ae2dbcce4
    name: drone-e2e-testing
    team: sarahconnor
    unit: demo
    owners:
      - ktruckenmiller@bellyjay1005.com
      - jadebello@bellyjay1005.com
    description: >
      end-to-end test cloudformation plugin
  environments:
    - name: qa
      cloud: aws
      account: 184518171237
      region: us-east-1
    - name: qa
      cloud: aws
      account: 184518171237
      region: us-east-2
---
kind: aws-cloudformation
spec:
  name: drone-e2e-testing
  template: template.yml
  parameters:
    Environment: qa
'''
    commit_sha = GITHUB.update_github_file(target_file, ref, message, content)
    assert commit_sha

def test_drone_job_triggered():
    '''
    Check if Drone job started
    '''
    build_numbers = DRONE.get_build_numbers()
    response_body = DRONE.get_build_content(build_numbers)
    assert response_body['started'] is not None

def test_get_drone_build_numbers():
    '''
    Get Drone build info and numbers
    '''
    build_numbers = DRONE.get_build_numbers()
    assert build_numbers is not None

def test_drone_response_status():
    '''
    Check Drone API response status
    '''
    build_number = DRONE.get_build_numbers()
    status = DRONE.get_build_response_status(build_number)
    assert status == 200

def test_drone_response_status_fail():
    '''
    Check Drone API response status
    '''
    build_number = 'fake-01'
    status = DRONE.get_build_response_status(build_number)
    assert status == 400

def test_drone_build_status():
    '''
    Check Drone job status
    '''
    build_number = DRONE.get_build_numbers()
    response_body = DRONE.get_build_content(build_number)
    assert response_body['status'] == 'success' or 'failure' or 'pending'

def test_drone_stage_status():
    '''
    Test Drone stages status
    '''
    build_number = DRONE.get_build_numbers()
    response_body = DRONE.get_build_content(build_number)
    assert response_body['stages'][0]['status'] == 'success' or 'failure'
    assert response_body['stages'][0]['steps'][0]['status'] == 'success' or 'failure'

def test_stop_drone_job():
    '''
    Test stopping specific Drone build job
    '''
    build_number = DRONE.get_build_numbers()
    assert DRONE.stop_drone_build_job(build_number)

def test_updating_drone_file():
    '''
    Update Drone file to delete stacks
    '''
    target_file = '.strithon.yml'
    ref = 'refs/heads/master'
    message = 'updated strithon-file to delete deployed e2e stacks'
    content = '''---
kind: service
metadata:
  service:
    id: e9337d78-d3d8-11e9-bb65-2a2ae2dbcce4
    name: drone-e2e-testing
    team: sarahconnor
    unit: demo
    owners:
      - ktruckenmiller@bellyjay1005.com
      - jadebello@bellyjay1005.com
    description: >
      end-to-end test cloudformation plugin
  environments:
    - name: qa
      cloud: aws
      account: 184518171237
      region: us-east-1
    - name: qa
      cloud: aws
      account: 184518171237
      region: us-east-2
---
kind: aws-cloudformation
spec:
  name: drone-e2e-testing
  template: template.yml
  parameters:
    Environment: qa
  state: delete
'''
    # wait for plugin to create stacks
    time.sleep(20)

    commit_sha = GITHUB.update_github_file(target_file, ref, message, content)
    assert commit_sha