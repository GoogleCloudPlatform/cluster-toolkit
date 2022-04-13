[manager]
%{for vm in omnia_manager ~}
${vm}
%{endfor}
[compute]
%{for vm in omnia_compute ~}
${vm}
%{endfor}
