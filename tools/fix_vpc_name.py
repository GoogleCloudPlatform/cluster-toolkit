import sys
import yaml
import re

def main():
    if len(sys.argv) != 4:
        print("Usage: python fix_vpc_name.py <blueprint_file> <vars_file> <prefix>")
        sys.exit(1)

    blueprint_file = sys.argv[1]
    vars_file = sys.argv[2]
    prefix = sys.argv[3]

    # Read test_name from vars file
    with open(vars_file, 'r') as f:
        vars_data = yaml.safe_load(f)
    
    test_name = vars_data.get('test_name')
    if not test_name:
        print(f"Error: test_name not found in {vars_file}")
        sys.exit(1)

    new_vpc_name = f"{prefix}-{test_name}"
    print(f"New VPC Name: {new_vpc_name}")

    # Read blueprint to find the network module ID
    with open(blueprint_file, 'r') as f:
        blueprint_data = yaml.safe_load(f)

    filestore_module_id = None
    network_module_id = None

    # Find filestore module and its network dependency
    for group in blueprint_data.get('deployment_groups', []):
        for module in group.get('modules', []):
            if module.get('source') == 'modules/file-system/filestore':
                filestore_module_id = module.get('id')
                uses = module.get('use', [])
                # Find a module in uses that is a VPC network
                for u in uses:
                    # Look for module with id 'u' in the blueprint
                    for g2 in blueprint_data.get('deployment_groups', []):
                        for m2 in g2.get('modules', []):
                            if m2.get('id') == u and m2.get('source') == 'modules/network/vpc':
                                network_module_id = u
                                break
                        if network_module_id:
                            break
                if network_module_id:
                    break
        if network_module_id:
            break

    if not network_module_id:
        print("Error: Could not find network module used by filestore")
        sys.exit(1)

    print(f"Found network module ID: {network_module_id}")

    # Now read the original file as text and modify it
    with open(blueprint_file, 'r') as f:
        content = f.read()

    lines = content.splitlines()
    new_lines = []
    in_network_module = False
    in_settings = False
    replaced = False
    indent_level = 0

    for line in lines:
        stripped = line.strip()
        if not in_network_module:
            new_lines.append(line)
            if stripped.startswith("- id:") and stripped.endswith(network_module_id):
                # Verify it matches exactly "- id: network_module_id"
                parts = stripped.split(":")
                if len(parts) == 2 and parts[1].strip() == network_module_id:
                    in_network_module = True
                    indent_level = len(line) - len(line.lstrip())
        else:
            # We are inside the network module block
            current_indent = len(line) - len(line.lstrip())
            
            if stripped.startswith("- id:") and current_indent == indent_level:
                # We hit the next module at the same indentation level
                if in_settings and not replaced:
                    new_lines.append(" " * (indent_level + 4) + f"network_name: {new_vpc_name}")
                    replaced = True
                in_network_module = False
                in_settings = False
                new_lines.append(line)
            elif stripped == "settings:":
                in_settings = True
                new_lines.append(line)
            elif in_settings and stripped.startswith("network_name:"):
                # Found it, replace it
                parts = line.split(":")
                indent = len(line) - len(line.lstrip())
                new_lines.append(" " * indent + f"network_name: {new_vpc_name}")
                replaced = True
                in_settings = False # assume only one network_name per settings
            else:
                new_lines.append(line)

    # Handle case where file ends while in network module
    if in_network_module and in_settings and not replaced:
         new_lines.append(" " * (indent_level + 4) + f"network_name: {new_vpc_name}")
         replaced = True

    if not replaced and in_network_module and not in_settings:
        # Found module but no settings, try to add settings
        print("Warning: Found module but no settings. Injecting settings.")
        # Find where to inject. Usually after 'source:' or 'id:'
        # Let's just append it to the module block if we can find the end of it.
        # This is getting complex. Let's assume settings exists for now as in the examples.
        pass

    if not replaced:
        print("Failed to replace network_name.")
        sys.exit(1)

    with open(blueprint_file, 'w') as f:
        f.write("\n".join(new_lines) + "\n")

    print("Successfully modified blueprint.")

if __name__ == "__main__":
    main()
