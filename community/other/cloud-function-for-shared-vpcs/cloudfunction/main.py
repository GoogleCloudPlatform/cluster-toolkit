import base64
import json
import os
import functions_framework
from google.cloud import compute_v1



@functions_framework.cloud_event
def process_log_entry(event):
    data_buffer = base64.b64decode(event.data["message"]["data"])
    log_entry = json.loads(data_buffer)["protoPayload"]

    host_project = os.getenv("HOST_PROJECT")    
    subnet_region = os.getenv("SUBNET_REGION")
    subnet_name = os.getenv("SUBNET_NAME")

    # Dont handle service accounts created by google.
    if not "principalEmail" in log_entry['authenticationInfo']:
      return
    
    client = compute_v1.SubnetworksClient()
    request = compute_v1.GetIamPolicySubnetworkRequest(
      project=host_project,
      region=subnet_region,
      resource=subnet_name,
    )

    iam_policy = client.get_iam_policy(request=request)

    members = []
    for o in iam_policy.bindings:
      members = [x for x in o.members if not x.startswith("deleted:")]
    if log_entry['methodName'] == 'google.iam.admin.v1.CreateServiceAccount':
      print("Adding " + log_entry['response']['email'] + " to list of authorized service accounts." )
      members.append("serviceAccount:" + log_entry['response']['email'])


    iam_policy.bindings[0].members = list(set(members))
    print("Current list of members", iam_policy.bindings[0].members)
    # Initialize request argument(s)
    request = compute_v1.SetIamPolicySubnetworkRequest(
      project=host_project,
      region=subnet_region,
      resource=subnet_name,
      region_set_policy_request_resource={"policy":iam_policy}
    )

    # Make the request
    response = client.set_iam_policy(request=request)
