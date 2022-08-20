[manager]
%{for vm in omnia_manager ~}
${vm}
%{endfor}
[compute]
%{for vm in omnia_compute ~}
${vm}
%{endfor}

[nfs_node]

[login_node]
