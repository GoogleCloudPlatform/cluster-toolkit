#Requires -RunAsAdministrator

setx http_proxy ${http_proxy} /m
setx https_proxy ${http_proxy} /m
setx no_proxy ${no_proxy} /m
