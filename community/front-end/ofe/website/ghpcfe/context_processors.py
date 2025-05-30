from ghpcfe.cluster_manager import utils

# Used to pass runtime_mode and is_local_mode to the templates so that we can add
# styling and logic based on whether or not the dev server is running.
def runtime_flags(request):
    return {
        'is_local_mode': utils.is_local_mode(),
        'runtime_mode': utils.load_config().get('server', {}).get('runtime_mode', ''),
    }
