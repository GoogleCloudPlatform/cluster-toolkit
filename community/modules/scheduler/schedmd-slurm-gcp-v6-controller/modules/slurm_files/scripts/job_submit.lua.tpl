SCRIPTS_DIR = "{scripts_dir}"
NO_VAL = 4294967294
-- get_tpu_vmcount.py exit code
PART_INVALID = -1 -- partition does not exists in config.yaml, thus do not exist in slurm
DIFF_VMCOUNTS_SAME_PART = -2 -- in the same partition there are nodesets with different vmcounts
DIFF_PART_DIFFERENT_VMCOUNTS = -3 -- partition is a list of partitions in which at least two of them have different vmcount
UNKWOWN_ERROR = -4 -- get_tpu_vmcount.py did not return a valid response

function get_part(job_desc, part_list)
    if job_desc.partition then
        return job_desc.partition
    end
    for name, val in pairs(part_list) do
        if val.flag_default == 1 then
            return name
        end
    end
    return nil
end

function os.capture(cmd, raw)
    local handle = assert(io.popen(cmd, 'r'))
    local output = assert(handle:read('*a'))
    handle:close()
    return output
end

function get_vmcount(part)
    local cmd = SCRIPTS_DIR .. "/get_tpu_vmcount.py -p " .. part
    local out = os.capture(cmd, true)
    for line in out:gmatch("(.-)\r?\n") do
        local tag, val = line:match("([^:]+):([^:]+)")
        if tag == "VMCOUNT" then
            return tonumber(val)
        end
    end
    return UNKWOWN_ERROR
end

function slurm_job_submit(job_desc, part_list, submit_uid)
    local part = get_part(job_desc, part_list)
    local vmcount = get_vmcount(part)
    -- Only do something if the job is in a TPU partition, if vmcount is 0, it implies that the partition(s) specified are not TPU ones
    if vmcount == 0 then
        return slurm.SUCCESS
    end
    -- This is a TPU job, but as the vmcount is 1 it can he handled the same way
    if vmcount == 1 then
        return slurm.SUCCESS
    end
    -- Check for errors
    if vmcount == PART_INVALID then
        slurm.log_user("Invalid partition specified " .. part)
        return slurm.FAILURE
    end
    if vmcount == DIFF_VMCOUNTS_SAME_PART then
        slurm.log_user("In partition(s) " .. part ..
                           " there are more than one tpu nodeset vmcount, this should not happen.")
        return slurm.ERROR
    end
    if vmcount == DIFF_PART_DIFFERENT_VMCOUNTS then
        slurm.log_user("In partition list " .. part ..
                           " there are more than one TPU types, cannot determine which is the correct vmcount to use, please retry with only one partition.")
        return slurm.FAILURE
    end
    if vmcount == UNKWOWN_ERROR then
        slurm.log_user("Something went wrong while executing get_tpu_vmcount.py.")
        return slurm.ERROR
    end
    -- This is surely a TPU node
    if vmcount > 1 then
        local min_nodes = job_desc.min_nodes
        local max_nodes = job_desc.max_nodes
        -- if not specified assume it is one, this should be improved taking into account the cpus, mem, and other factors
        if min_nodes == NO_VAL then
            min_nodes = 1
            max_nodes = 1
        end
        -- as max_nodes can be higher than the nodes in the partition, we are not able to calculate with certainty the nodes that this job will have if this value is set to something
        -- different than min_nodes
        if min_nodes ~= max_nodes then
            slurm.log_user("Max nodes cannot be set different than min nodes for the TPU partitions.")
            return slurm.ERROR
        end
        -- Set the number of switches to the number of nodes originally requested by the job, as the job requests "TPU groups"
        job_desc.req_switch = min_nodes

        -- Apply the node increase into the job description.
        job_desc.min_nodes = min_nodes * vmcount
        job_desc.max_nodes = max_nodes * vmcount
        -- if job_desc.features then
        -- slurm.log_user("Features: %s",job_desc.features)
        -- end
    end

    return slurm.SUCCESS
end

function slurm_job_modify(job_desc, job_rec, part_list, modify_uid)
    return slurm.SUCCESS
end

return slurm.SUCCESS
