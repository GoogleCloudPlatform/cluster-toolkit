#!/usr/bin/env python3
# Copyright 2024 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

'''
This is a backend part needed for importing existing images into
Frontend views will talk with functions here to perform real actions.
'''

def get_credentials(credential_info):
    '''
    Use the OFE credential info to create an oauth credential object
    '''
    from google.oauth2 import service_account
    import json
    cred_id = credential_info.__dict__["id"]
    cred_detail = credential_info.__dict__["detail"]
    cred_detail_json = json.loads(cred_detail)
    credentials = service_account.Credentials.from_service_account_info(cred_detail_json)
    return credentials

def get_project_from_credentials(credential_info):
    '''
    Return the project name from a credential object
    '''
    import json
    cred_detail = credential_info.__dict__["detail"]
    cred_detail_json = json.loads(cred_detail)
    return cred_detail_json["project_id"]     


def get_images_info(credentials, project,cred_id=False):
    '''
    Retrieve the list of images in a given project using provided credential
    '''
    from google.cloud import compute_v1
    client = compute_v1.ImagesClient(credentials=credentials)
    request = compute_v1.ListImagesRequest(project=project)
    image_response = client.list(request)
    existing_image_list = []
    for image in image_response.items:
        existing_image_list.append([cred_id,image.name, image.description, image.family])
    return existing_image_list


def list_project_images(credential_info):
    '''
    Return a list of compute images from a credential object
    '''
    credentials = get_credentials(credential_info)
    project = get_project_from_credentials(credential_info)
    images_info = get_images_info(credentials, project, credential_info.__dict__["id"])
    return images_info


def verify_image(credential_info,image_name, image_family):
    '''
    Verify that an image with input name/family actually exists in GCP 
    with specified credential
    '''
    project = get_project_from_credentials(credential_info)
    credentials = get_credentials(credential_info)
    images_list = get_images_info(credentials,project)
    found_img = False
    for img in images_list:
         cloud_img_name = img[1]
         cloud_img_fam = img[3]
         if image_name == cloud_img_name and image_family == cloud_img_fam:
              found_img = True
    return found_img
