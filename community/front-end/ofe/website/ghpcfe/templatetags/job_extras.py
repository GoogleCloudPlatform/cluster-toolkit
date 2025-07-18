# Copyright 2025 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import json
import logging
from django import template
from urllib.parse import urlencode

register = template.Library()
logger = logging.getLogger(__name__)


@register.filter
def job_success_status(job):
    """Return a human-readable status for the job, using SLURM info if available, else OFE status."""
    slurm_status = getattr(job, 'slurm_status', None)
    exit_code = getattr(job, 'slurm_exit_code', None)
    if slurm_status:
        if slurm_status == 'COMPLETED':
            # Use exit code to determine if it was really successful
            if exit_code is not None:
                try:
                    import ast
                    if isinstance(exit_code, str):
                        exit_code_obj = ast.literal_eval(exit_code)
                    else:
                        exit_code_obj = exit_code
                    code = None
                    if isinstance(exit_code_obj, dict):
                        code = exit_code_obj.get('return_code', {}).get('number', None)
                    if code is not None and code != 0:
                        return 'Completed with Errors'
                    else:
                        return 'Completed Successfully'
                except Exception:
                    return 'Completed (unknown exit code)'
            else:
                return 'Completed Successfully'
        elif slurm_status == 'RUNNING':
            return 'Running'
        elif slurm_status == 'FAILED':
            return 'Completed with Errors'
        elif slurm_status == 'CANCELLED':
            return 'Cancelled'
        else:
            return slurm_status.title()
    # Fallback to OFE status
    if hasattr(job, 'get_status_display'):
        return job.get_status_display()
    return 'Unknown'


@register.filter
def job_success_badge_class(job):
    """Return a Bootstrap badge class for the job's SLURM status and exit code."""
    slurm_status = getattr(job, 'slurm_status', None)
    exit_code = getattr(job, 'slurm_exit_code', None)
    if slurm_status:
        if slurm_status == 'COMPLETED':
            if exit_code is not None:
                try:
                    import ast
                    if isinstance(exit_code, str):
                        exit_code_obj = ast.literal_eval(exit_code)
                    else:
                        exit_code_obj = exit_code
                    code = None
                    if isinstance(exit_code_obj, dict):
                        code = exit_code_obj.get('return_code', {}).get('number', None)
                    if code is not None and code != 0:
                        return 'bg-danger'
                    else:
                        return 'bg-success'
                except Exception:
                    return 'bg-secondary'
            else:
                return 'bg-success'
        elif slurm_status == 'RUNNING':
            return 'bg-warning text-dark'
        elif slurm_status == 'FAILED':
            return 'bg-danger'
        elif slurm_status == 'CANCELLED':
            return 'bg-secondary'
        else:
            return 'bg-secondary'
    # Fallback to OFE status
    if hasattr(job, 'status'):
        if job.status == 'c':
            return 'bg-success'
        elif job.status == 'e':
            return 'bg-danger'
        elif job.status == 'r':
            return 'bg-warning text-dark'
    return 'bg-secondary'


@register.filter
def job_exit_code_display(job):
    """Return a human-readable exit code display"""
    if not job.slurm_exit_code:
        return "N/A"

    try:
        exit_code = job.slurm_exit_code
        
        # First, try to parse as JSON if it's a string
        if isinstance(exit_code, str):
            try:
                # Try to parse as JSON first
                exit_code = json.loads(exit_code)
            except (ValueError, TypeError):
                # If JSON parsing fails, try to parse as Python literal (for dict strings)
                try:
                    import ast
                    exit_code = ast.literal_eval(exit_code)
                except (ValueError, SyntaxError):
                    pass  # Not JSON or Python literal, treat as regular string
        
        if isinstance(exit_code, dict):
            # Complex JSON format from SLURM
            return_code_data = exit_code.get('return_code', {})
            signal_data = exit_code.get('signal', {})

            if isinstance(return_code_data, dict) and 'number' in return_code_data:
                exit_num = return_code_data['number']
                signal_num = 0

                # Get signal number if available
                if isinstance(signal_data, dict):
                    signal_id_data = signal_data.get('id', {})
                    if isinstance(signal_id_data, dict) and 'number' in signal_id_data:
                        signal_num = signal_id_data['number']

                if signal_num == 0 and exit_num == 0:
                    return "Success (0)"
                elif signal_num == 0:
                    return f"Exit Code {exit_num}"
                else:
                    return f"Signal {signal_num}, Exit {exit_num}"
            else:
                # Fallback for other dict formats
                return str(job.slurm_exit_code)
        elif isinstance(exit_code, str):
            if ':' in exit_code:
                signal, code = exit_code.split(':', 1)
                signal_num = int(signal)
                exit_num = int(code)

                if signal_num == 0 and exit_num == 0:
                    return "Success (0)"
                elif signal_num == 0:
                    return f"Exit Code {exit_num}"
                else:
                    return f"Signal {signal_num}, Exit {exit_num}"
            else:
                exit_num = int(exit_code)
                if exit_num == 0:
                    return "Success (0)"
                else:
                    return f"Exit Code {exit_num}"
        else:
            # Handle other formats
            exit_num = int(exit_code)
            if exit_num == 0:
                return "Success (0)"
            else:
                return f"Exit Code {exit_num}"
    except (ValueError, TypeError, KeyError):
        return str(job.slurm_exit_code)


@register.simple_tag(takes_context=True)
def pagination_url(context, page_number):
    """Generate a pagination URL that preserves all current filters and parameters"""
    request = context.get('request')
    if not request:
        return f"?page={page_number}"

    # Get all current parameters
    params = request.GET.copy()

    # Update the page number
    params['page'] = page_number

    # Build the URL
    return f"?{urlencode(params)}" 
