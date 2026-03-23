import yaml
import sys

def modify_vpcs(blueprint_path):
    with open(blueprint_path, 'r') as f:
        data = yaml.safe_load(f)

    vpc_count = 0
    # Iterate through all deployment groups and modules
    if 'deployment_groups' in data:
        for group in data['deployment_groups']:
            for module in group.get('modules', []):
                # Identify VPC modules by their source path
                if 'modules/network/vpc' in module.get('source', ''):
                    if 'settings' not in module:
                        module['settings'] = {}
                    # Set the consecutive name using the $(vars.test_name) variable
                    module['settings']['network_name'] = f"$(vars.test_name)-{vpc_count}"
                    print(f"Updated module '{module.get('id')}' to: $(vars.test_name)-{vpc_count}")
                    vpc_count += 1

    with open(blueprint_path, 'w') as f:
        yaml.dump(data, f, sort_keys=False)

if __name__ == "__main__":
    if len(sys.argv) < 2:
        sys.exit(1)
    modify_vpcs(sys.argv[1])

