In this setup we are connecting an on-prem network(192.168.1.0/24) with a gcp network(10.0.0.0/24) using the ipsec vpn strongswan.
If you have an enterprise router, like cisco pfsense or similar, most probably there is already an ipsec vpn solution that can be integrated with GCP vpn.
For this example we are using on-prem a debian 12 VM in order to keep things separated. Most probably the vpn can be installed and configured in the same deployment machine, but this setup has not been tested.
### GCP Static IP
Crete a GCP static IP address and get the actual IP address:

```shell
gcloud compute addresses create “gcp-vpn-ip” --region=$REGION
gcloud compute addresses describe "gcp-vpn-ip" --region=$REGION --format='value(address)' 
```

We will reference that IP address as “GCP_VPN_IP”

### On-prem vpn server

In on-prem get a debian VM to execute the next script, change the first block with the appropriate data:

ONPREM_NETWORK is your on premise network
GCP_NETWORK is the subnetwork in GCP, if you use a regular toolkit vpc module that 10.0.0.0/24 should work for you.
GCP_VPN_IP is the IP address we created in the previous step.
ONPREM_PUBLIC_IP is calculated, if the curl option does not work for you, set it manually.

```shell
ONPREM_NETWORK=192.168.1.0/24
GCP_NETWORK=10.0.0.0/24
GCP_VPN_IP=45.34.12.45


ONPREM_PUBLIC_IP=$(curl ifconfig.me)


cat >> /etc/sysctl.conf << EOF
net.ipv4.ip_forward = 1
net.ipv4.conf.all.accept_redirects = 0
net.ipv4.conf.all.send_redirects = 0
EOF
sysctl -p /etc/sysctl.conf
apt update && apt upgrade -y && apt install strongswan -y
KEY=$(openssl rand -base64 32)
cat >> /etc/ipsec.secrets << EOF
# This file holds shared secrets or RSA private keys for authentication.
$ONPREM_PUBLIC_IP $GCP_VPN_IP : PSK "$KEY"
EOF
cat >> /etc/ipsec.conf << EOF
config setup
       charondebug="all"
       uniqueids=yes
       strictcrlpolicy=no


conn onprem-to-gcp
 authby=secret
 left=%defaultroute
 leftid=$ONPREM_PUBLIC_IP
 leftsubnet=$ONPREM_NETWORK
 right=$GCP_VPN_IP
 rightsubnet=$GCP_NETWORK
 ike=aes256-sha2_256-modp1024!
 esp=aes256-sha2_256!
 keyingtries=0
 ikelifetime=1h
 lifetime=8h
 dpddelay=30
 dpdtimeout=120
 dpdaction=restart
 auto=start
EOF
echo "your KEY is: $KEY"
```

Take a note of the KEY, we will use it later.

### Prepare on-prem router

In your on-prem router ensure these are set, if your router is also the vpn machine you might not need these:
Port forward 500 and 4500 UDP to vpn machine
Allow esp packets (protocol number 50)
Add route so that $GCP_NETWORK goes through the vpn machine

### Create GCP vpn server and tunnel

Create the GCP vpn tunnel, change all the values needed in the CONFIG section

```shell
#CONFIG
GCP_PROJ="YOUR_GCP_PROJECT_NAME"
REGION="REGION_YOU_CHOOSE" #Use the same regions as the computes nodes will be, this ensure the minimum latency.
VPC_NETWORK_NAME="slurm-hybrid-net" #match this with the one created by gcluster, if using the example this is correct.
VPC_SUBNET="10.0.0.0/24" # same as GCP_NETWORK in step 2, modify this accordingly to what your subnetwork looks like.
ONPREM_NETWORK="192.168.1.0/24" #same as ONPREM_NETWORK in step 2.
SHARED_SECRET="MY_SUPER_SECRET_KEY" #the KEY we noted down at step 2
GCP_VPN_IP_NAME="gcp-vpn-ip" #the name of the static address we requested in step 1


ON_PREM_PUBLIC_IP=$(curl -s ifconfig.me) #set that to the same value that has been obtained in step 2


#NAMES, you can customize this as you want
VPN_GATEWAY_NAME="gcp-vpn-gateway"
TUNNEL_NAME="tunnel-to-onprem"
ROUTE_TO_ONPREM="route-to-onprem"
FW_RULE_NAME="allow-all-from-onprem"


#VPN Gateway
gcloud compute target-vpn-gateways create $VPN_GATEWAY_NAME --network=$VPC_NETWORK_NAME --region=$REGION --project=$GCP_PROJ
#forwarding rules
##ESP
gcloud compute forwarding-rules create fr-esp --region=$REGION --ip-protocol=ESP --address=$GCP_VPN_IP_NAME --target-vpn-gateway=$VPN_GATEWAY_NAME --project=$GCP_PROJ
##UDP 500
gcloud compute forwarding-rules create fr-udp500 --region=$REGION --ip-protocol=UDP --ports=500 --address=$GCP_VPN_IP_NAME --target-vpn-gateway=$VPN_GATEWAY_NAME --project=$GCP_PROJ
##UDP 4500
gcloud compute forwarding-rules create fr-udp4500 --region=$REGION --ip-protocol=UDP --ports=4500 --address=$GCP_VPN_IP_NAME --target-vpn-gateway=$VPN_GATEWAY_NAME --project=$GCP_PROJ
#VPN Tunnel
gcloud compute vpn-tunnels create $TUNNEL_NAME --region=$REGION --target-vpn-gateway=$VPN_GATEWAY_NAME --peer-address=$ON_PREM_PUBLIC_IP --ike-version=2 --shared-secret="$SHARED_SECRET" --local-traffic-selector=0.0.0.0/0 --remote-traffic-selector=0.0.0.0/0 --project=$GCP_PROJ
#route to on-premises network
gcloud compute routes create $ROUTE_TO_ONPREM --network=$VPC_NETWORK_NAME --next-hop-vpn-tunnel=$TUNNEL_NAME --next-hop-vpn-tunnel-region=$REGION --destination-range=$ONPREM_NETWORK --project=$GCP_PROJ
#FW rule to allow on-prem
gcloud compute firewall-rules create $FW_RULE_NAME --direction=INGRESS --priority=1000 --network=$VPC_NETWORK_NAME --action=ALLOW --rules=all --source-ranges=$ONPREM_NETWORK --project=$GCP_PROJ
```

### Restart the tunnel on-prem

Go back to the vpn machine and execute

```shell
ipsec restart
```

This will restart to tunnel to ensure that is connected to the newly created gcp infrastructure.
Wait a few moments and the tunnel should be up. You can check the status with ipsec status. Also you can spin up a vm in your subnetwork and ping some IPs of your onpremise network.

### Delete the cloud infrastructure

In order to remove all the gcp infrastructure for the VPN when you are done with it you could use these commands(modify them to your names and config):

```shell
#!/bin/bash
set -e

# CONFIG
GCP_PROJ="YOUR_GCP_PROJECT_NAME"
REGION="REGION_YOU_CHOOSE"

# NAMES
GCP_VPN_IP_NAME="gcp-vpn-ip"
VPN_GATEWAY_NAME="gcp-vpn-gateway"
TUNNEL_NAME="tunnel-to-onprem"

# Delete firewall rule
gcloud compute firewall-rules delete allow-all-from-onprem --quiet --project=$GCP_PROJ

# Delete route
gcloud compute routes delete route-to-onprem --quiet --project=$GCP_PROJ

# Delete VPN tunnel
gcloud compute vpn-tunnels delete $TUNNEL_NAME --region=$REGION --quiet --project=$GCP_PROJ

# Delete forwarding rules
gcloud compute forwarding-rules delete fr-esp --region=$REGION --quiet --project=$GCP_PROJ
gcloud compute forwarding-rules delete fr-udp500 --region=$REGION --quiet --project=$GCP_PROJ
gcloud compute forwarding-rules delete fr-udp4500 --region=$REGION --quiet --project=$GCP_PROJ

# Delete VPN gateway
gcloud compute target-vpn-gateways delete $VPN_GATEWAY_NAME --region=$REGION --quiet --project=$GCP_PROJ

# Delete static IP
gcloud compute addresses delete $GCP_VPN_IP_NAME --region=$REGION
```
