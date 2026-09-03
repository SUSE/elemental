#!/bin/bash

set -euo pipefail

command -v aws >/dev/null 2>&1 || { echo "AWS CLI is not installed" >&2; exit 1; }

export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-eu-central-1}"
aws sts get-caller-identity >/dev/null 2>&1 || { echo "AWS CLI is not configured" >&2; exit 1; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
: "${SHARED_ENVS:=$SCRIPT_DIR/shared.env}"
: "${RUN_ID:=$(date +%s)}"

# Determine whether provided IP is available from the subnet range.
is_ip_available() {
    local ip="$1"
    local eni_id

    eni_id="$(aws ec2 describe-network-interfaces \
        --filters \
            "Name=subnet-id,Values=$SUBNET_ID" \
            "Name=addresses.private-ip-address,Values=$ip" \
        --query 'NetworkInterfaces[0].NetworkInterfaceId' \
        --output text)"

    [[ "$eni_id" == "None" ]]
}

# Populate an array with a specified number free IPs starting from a given IP number
populate_arr_with_ips() {
    local -n outside_arr="$1"
    local start_addr="$2"
    local num_addr="$3"
    local i ip

    for ((i=start_addr; i<=254; i++)); do
        ip="$SUBNET_BASE.$i"
        is_ip_available "$ip" && outside_arr+=("$ip")
        (( ${#outside_arr[@]} == num_addr )) && return 0
    done

    echo "Could not find $num_addr free IPs from $SUBNET_BASE.$start_addr" >&2
    return 1
}

# Setup default VPC if missing, and prepare the static IPs for the internal NLB, control-plane and worker nodes.
setup_network() {
    echo "|--------------------Network---------------------|"
    VPC_ID="$(aws ec2 describe-vpcs \
        --filters Name=isDefault,Values=true \
        --query 'Vpcs[0].VpcId' \
        --output text)"

    [[ "$VPC_ID" == "None" ]] && {
        VPC_ID="$(aws ec2 create-default-vpc \
            --query 'Vpc.VpcId' \
            --output text)"
    }
    echo "VPC ID: $VPC_ID"
    
    VPC_CIDR="$(aws ec2 describe-vpcs --vpc-ids "$VPC_ID" --query 'Vpcs[].CidrBlock' --output text)"
    echo "Default VPC primary CIDR: $VPC_CIDR"

    SUBNET_ID="$(aws ec2 describe-subnets \
        --filters Name=vpc-id,Values="$VPC_ID" Name=default-for-az,Values=true \
        --query 'Subnets[0].SubnetId' --output text)"

    echo "SUBNET_ID=$SUBNET_ID" >> "$SHARED_ENVS"

    SUBNET_CIDR="$(aws ec2 describe-subnets \
        --subnet-ids "$SUBNET_ID" \
        --query 'Subnets[].CidrBlock' \
        --output text)"

    SUBNET_BASE="${SUBNET_CIDR%.*}"

    TOTAL_DESIRED_IPS_NUM=$((STATIC_CP_IPS + STATIC_WK_IPS + 1))
    ALL_IPS=()
    populate_arr_with_ips ALL_IPS 10 "$TOTAL_DESIRED_IPS_NUM"

    CONTROL_VIP="${ALL_IPS[0]}"
    CP_NODE_IPS=("${ALL_IPS[@]:1:STATIC_CP_IPS}")
    WK_NODE_IPS=("${ALL_IPS[@]:$((STATIC_CP_IPS + 1)):STATIC_WK_IPS}")
    NODE_IPS=("${CP_NODE_IPS[@]}" "${WK_NODE_IPS[@]}")

    echo "NODE_IPS=\"${NODE_IPS[*]}\"" >> "$SHARED_ENVS"
    echo "Base CIDR used for static IP generation: $SUBNET_CIDR"
    echo "Static control-plane IPs: ${CP_NODE_IPS[*]}"
    echo "Static worker IPs: ${WK_NODE_IPS[*]}"
    echo "Static IP for internal cluster NLB: $CONTROL_VIP"
}

# Create a security group with a given name and description.
create_security_group() {
    local name="$1"
    local description="$2"

    aws ec2 create-security-group \
        --group-name "${name}-${RUN_ID}" \
        --description "$description" \
        --vpc-id "$VPC_ID" \
        --query 'GroupId' \
        --output text
}

# Setup security groups and authorizations for the RKE2 nodes, internal and internet-facing NLBs.
setup_security_groups() {
    echo "|--------------------Security--------------------|"
    NODE_SG="$(create_security_group rke2-nodes "RKE2 nodes")"
    echo "NODE_SG=$NODE_SG" >> "$SHARED_ENVS"

    CONTROL_NLB_SG="$(create_security_group rke2-control-nlb "RKE2 control NLB")"
    echo "CONTROL_NLB_SG=$CONTROL_NLB_SG" >> "$SHARED_ENVS"

    INGRESS_NLB_SG="$(create_security_group rke2-ingress-nlb "RKE2 ingress NLB")"
    echo "INGRESS_NLB_SG=$INGRESS_NLB_SG" >> "$SHARED_ENVS"

    echo "Security group ID for RKE2 nodes:       $NODE_SG"
    echo "Security group ID for RKE2 control NLB: $CONTROL_NLB_SG"
    echo "Security group ID for RKE2 ingress NLB: $INGRESS_NLB_SG"

    echo "Authorizing inbound traffic between RKE2 nodes using the $NODE_SG SG"
    aws ec2 authorize-security-group-ingress \
        --group-id "$NODE_SG" \
        --protocol all \
        --source-group "$NODE_SG" \
        >/dev/null

    if [[ "$ALLOW_SSH" == true ]]; then
        SSH_CIDR="$(curl -fsS https://checkip.amazonaws.com)/32"

        echo "Authorizing $SSH_CIDR as an allowed IP to SSH to the RKE2 nodes"
        aws ec2 authorize-security-group-ingress \
            --group-id "$NODE_SG" \
            --protocol tcp \
            --port 22 \
            --cidr "$SSH_CIDR" \
            >/dev/null
    fi
    
    echo "Authorizing TCP connections to '6443' and '9345' from $VPC_CIDR"
    for port in 6443 9345; do
        aws ec2 authorize-security-group-ingress \
            --group-id "$CONTROL_NLB_SG" \
            --protocol tcp \
            --port "${port}" \
            --cidr "$VPC_CIDR" \
            >/dev/null

        aws ec2 authorize-security-group-ingress \
            --group-id "$NODE_SG" \
            --protocol tcp \
            --port "${port}" \
            --source-group "$CONTROL_NLB_SG" \
            >/dev/null
    done

    echo "Authorizing TCP connections to '80' and '443' only for traffic associated with the $INGRESS_NLB_SG SG"
    for port in 80 443; do
        aws ec2 authorize-security-group-ingress \
            --group-id "$INGRESS_NLB_SG" \
            --protocol tcp \
            --port "${port}" \
            --cidr 0.0.0.0/0 \
            >/dev/null

        aws ec2 authorize-security-group-ingress \
            --group-id "$NODE_SG" \
            --protocol tcp \
            --port "${port}" \
            --source-group "$INGRESS_NLB_SG" \
            >/dev/null
    done
}

# Create a target group with the given name and port and assign it to the default VPC.
create_target_group() {
    local name="$1"
    local port="$2"

    aws elbv2 create-target-group \
        --name "${name}-${RUN_ID}" \
        --protocol TCP \
        --port "${port}" \
        --vpc-id "$VPC_ID" \
        --target-type ip \
        --health-check-protocol TCP \
        --query 'TargetGroups[0].TargetGroupArn' \
        --output text
}

# Setup target groups for the RKE2 API server and supervisor,
# as well as for HTTP and HTTPS ingress traffic. Binds created
# target groups to the static IPs configured for control-plane
# and worker nodes.
setup_target_groups() {
    echo "|---------------------Target---------------------|"
    API_TG="$(create_target_group rke2-api 6443)"
    echo "API_TG=$API_TG" >> "$SHARED_ENVS"

    SUPERVISOR_TG="$(create_target_group rke2-sup 9345)"
    echo "SUPERVISOR_TG=$SUPERVISOR_TG" >> "$SHARED_ENVS"

    HTTP_INGRESS_TG="$(create_target_group rke2-http 80)"
    echo "HTTP_INGRESS_TG=$HTTP_INGRESS_TG" >> "$SHARED_ENVS"

    HTTPS_INGRESS_TG="$(create_target_group rke2-https 443)"
    echo "HTTPS_INGRESS_TG=$HTTPS_INGRESS_TG" >> "$SHARED_ENVS"

    echo "ARN for RKE2 API target group: $API_TG"
    echo "ARN for RKE2 Supervisor target group: $SUPERVISOR_TG"
    echo "ARN for RKE2 ingress HTTP target group: $HTTP_INGRESS_TG"
    echo "ARN for RKE2 ingress HTTPS target group: $HTTPS_INGRESS_TG"

    # Allow for a 30 seconds timeout to drain existing connections when the target
    # is removed from the target group. Mainly needed for faster cleanup later on.
    for TG in "$API_TG" "$SUPERVISOR_TG" "$HTTP_INGRESS_TG" "$HTTPS_INGRESS_TG"; do
        aws elbv2 modify-target-group-attributes \
            --target-group-arn "$TG" \
            --attributes Key=deregistration_delay.timeout_seconds,Value=30 \
            >/dev/null
    done

    local cp_targets=()
    local ingress_targets=()
    local ip

    for ip in "${CP_NODE_IPS[@]}"; do
        cp_targets+=("Id=$ip")
    done

    for ip in "${NODE_IPS[@]}"; do
        ingress_targets+=("Id=$ip")
    done

    echo "Registering ${CP_NODE_IPS[*]} to RKE2 API and Supervisor TGs"
    for TG in "$API_TG" "$SUPERVISOR_TG"; do
        aws elbv2 register-targets \
            --target-group-arn "$TG" \
            --targets "${cp_targets[@]}" \
            >/dev/null
    done

    echo "Registering ${NODE_IPS[*]} to HTTP and HTTPS ingress TGs"
    for TG in "$HTTP_INGRESS_TG" "$HTTPS_INGRESS_TG"; do
        aws elbv2 register-targets \
            --target-group-arn "$TG" \
            --targets "${ingress_targets[@]}" \
            >/dev/null
    done
}

# Create a NLB listener for a specific NLB, listening on a specific port
# bound to a specific target group.
create_nlb_listener() {
    local nlb_arn="$1"
    local port="$2"
    local tg_arn="$3"

    aws elbv2 create-listener \
        --load-balancer-arn "$nlb_arn" \
        --protocol TCP \
        --port "$port" \
        --default-actions "Type=forward,TargetGroupArn=${tg_arn}" \
        >/dev/null
}

# Setup NLB for internet facing communication on ports 80 and 443 and assign an EIP to it.
# Setup NLB for internal node communication on ports 6443 and 9345.
setup_nlbs() {
    echo "|------------------Loadbalancer------------------|"

    EIP_OUTPUT="$(aws ec2 allocate-address \
        --domain vpc \
        --query '[PublicIp,AllocationId]' \
        --output text)"

    read -r INGRESS_EIP INGRESS_EIP_ALLOC_ID <<< "$EIP_OUTPUT"
    [[ -n "$INGRESS_EIP" && -n "$INGRESS_EIP_ALLOC_ID" ]] || {
        echo "Failed to allocate public IP for internet-facing NLB" >&2
        exit 1
    }

    echo "INGRESS_EIP_ALLOC_ID=$INGRESS_EIP_ALLOC_ID" >> "$SHARED_ENVS"
    echo "Allocated public IP for internet-facing NLB: $INGRESS_EIP"

    INGRESS_LB_ARN="$(aws elbv2 create-load-balancer --name "rke2-ingress-${RUN_ID}" \
        --type network --scheme internet-facing \
        --subnet-mappings "SubnetId=$SUBNET_ID,AllocationId=$INGRESS_EIP_ALLOC_ID" \
        --security-groups "$INGRESS_NLB_SG" \
        --query 'LoadBalancers[0].LoadBalancerArn' \
        --output text)"
    echo "INGRESS_LB_ARN=$INGRESS_LB_ARN" >> "$SHARED_ENVS"
    echo "Created internet-facing NLB for HTTP and HTTPS ingress communication: $INGRESS_LB_ARN"

    CONTROL_LB_ARN="$(aws elbv2 create-load-balancer --name "rke2-control-${RUN_ID}" \
        --type network --scheme internal \
        --subnet-mappings "SubnetId=$SUBNET_ID,PrivateIPv4Address=$CONTROL_VIP" \
        --security-groups "$CONTROL_NLB_SG" \
        --query 'LoadBalancers[0].LoadBalancerArn' \
        --output text)"
    echo "CONTROL_LB_ARN=$CONTROL_LB_ARN" >> "$SHARED_ENVS"
    echo "Created internal NLB for RKE2 API and Supervisor communication: $CONTROL_LB_ARN"

    echo "Waiting for NLB availability. This may take some time..."
    aws elbv2 wait load-balancer-available --load-balancer-arns "$CONTROL_LB_ARN" "$INGRESS_LB_ARN"

    echo "Creating listeners for the newly created NLBs"
    create_nlb_listener "$CONTROL_LB_ARN" 6443 "$API_TG"
    create_nlb_listener "$CONTROL_LB_ARN" 9345 "$SUPERVISOR_TG"
    create_nlb_listener "$INGRESS_LB_ARN" 80   "$HTTP_INGRESS_TG"
    create_nlb_listener "$INGRESS_LB_ARN" 443  "$HTTPS_INGRESS_TG"

    # Retrieve the internal NLB host so that it can be outputted to the user.
    CONTROL_NLB_HOST="$(aws elbv2 describe-load-balancers \
        --load-balancer-arns "$CONTROL_LB_ARN" \
        --query 'LoadBalancers[0].DNSName' \
        --output text)"

    echo "Internet facing NLB public IP address: $INGRESS_EIP"
    echo "Internal NLB host: $CONTROL_NLB_HOST"
    echo "Internal NLB IP: $CONTROL_VIP"
}

# Wrapper command for the bootstrap_infra workflow.
bootstrap_infra() {
    local ALLOW_SSH="false"
    local STATIC_CP_IPS=3
    local STATIC_WK_IPS=1

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --allow-ssh)
                ALLOW_SSH="true"
                shift
                ;;
            --static-cp-ips)
                [[ $# -ge 2 ]] || { echo "--static-cp-ips requires a value" >&2; exit 1; }
                [[ "$2" =~ ^[0-9]+$ ]] || { echo "--static-cp-ips value must hold a non-negative value" >&2; exit 1; }
                STATIC_CP_IPS="$2"
                shift 2
                ;;
            --static-wk-ips)
                [[ $# -ge 2 ]] || { echo "--static-wk-ips requires a value" >&2; exit 1; }
                [[ "$2" =~ ^[0-9]+$ ]] || { echo "--static-wk-ips value must hold a non-negative value" >&2; exit 1; }
                STATIC_WK_IPS="$2"
                shift 2
                ;;
            *)
                echo "Unknown option: $1" >&2
                exit 1
                ;;
        esac
    done

    setup_network
    setup_security_groups
    setup_target_groups
    setup_nlbs
}

# Setup policy allowing for VM import/export service to access the 'vmimport' IAM role.
vmimport_trust_policy() {
    cat <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": { "Service": "vmie.amazonaws.com" },
    "Action": "sts:AssumeRole",
    "Condition": { "StringEquals": { "sts:ExternalId": "vmimport" } }
  }]
}
EOF
}

# Setup policy to allow for accessing matching S3 buckets and managing
# EC2 snapshot/AMI. Needed so that the .raw image in the S3 bucket
# can be converted to an AMI from which an EC2 instance will be started.
vmimport_role_policy() {
    cat <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:GetBucketLocation", "s3:ListBucket"],
      "Resource": "arn:aws:s3:::${BUCKET}*"
    },
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject"],
      "Resource": "arn:aws:s3:::${BUCKET}*/*"
    },
    {
      "Effect": "Allow",
      "Action": ["ec2:ModifySnapshotAttribute", "ec2:CopySnapshot",
                 "ec2:RegisterImage", "ec2:Describe*"],
      "Resource": "*"
    }
  ]
}
EOF
}

# Ensure that the 'vmimport' role exists and has the required policy.
setup_vmimport_role() {
    if ! aws iam get-role --role-name vmimport >/dev/null 2>&1; then
        echo "Creating 'vmimport' role"
        aws iam create-role \
            --role-name vmimport \
            --assume-role-policy-document "$(vmimport_trust_policy)"
    fi

    echo "Creating/updating 'vmimport' policy"
    aws iam put-role-policy \
        --role-name vmimport \
        --policy-name elemental-vmimport-s3 \
        --policy-document "$(vmimport_role_policy)"
}

# Setup the S3 bucket as defined in BUCKET if it does not already exist.
setup_s3_bucket() {
    echo "|--------------------S3 Bucket-------------------|"

    if aws s3api head-bucket --bucket "$BUCKET" >/dev/null 2>&1; then
        echo "S3 bucket already exists: $BUCKET"
        return 0
    fi

    echo "Creating S3 bucket: $BUCKET"
    aws s3api create-bucket \
        --bucket "$BUCKET" \
        --region "$AWS_DEFAULT_REGION" \
        --create-bucket-configuration "LocationConstraint=$AWS_DEFAULT_REGION" \
        >/dev/null

    echo "S3 bucket created: $BUCKET"
}

# Transfers a given image to the UC S3 bucket at the PATH_IN_BUCKET location.
transfer_img_to_s3() {
    local img="$1"

    echo "Transferring ${img} to $BUCKET bucket"
    aws s3 cp "${img}" "s3://$BUCKET/$PATH_IN_BUCKET"
}

# Creates an EBS snapshot from the image located in the BUCKET bucker under the
# PATH_IN_BUCKET path.
import_snapshot() {
    echo "|--------------------Snapshot--------------------|"
    TASK_ID="$(aws ec2 import-snapshot \
        --description "$AMI_NAME" \
        --disk-container "Format=RAW,UserBucket={S3Bucket=$BUCKET,S3Key=$PATH_IN_BUCKET}" \
        --query ImportTaskId --output text)"

    echo "Waiting for snapshot import. This will take some time.."
    printf '%-12s %-10s %s\n' "STATUS" "PROGRESS" "MESSAGE"
    while true; do
        IFS=$'\t' read -r STATUS PROGRESS MESSAGE <<< "$(aws ec2 describe-import-snapshot-tasks \
            --import-task-ids "$TASK_ID" \
            --query 'ImportSnapshotTasks[0].SnapshotTaskDetail.[Status,Progress,StatusMessage]' \
            --output text)"

        printf '%-12s %-10s %s\n' "$STATUS" "$PROGRESS" "$MESSAGE"

        case "$STATUS" in
            completed) break ;;
            deleted)   echo "import failed" >&2; return 1 ;;
        esac

        sleep 30
    done

    SNAP="$(aws ec2 describe-import-snapshot-tasks \
            --import-task-ids "$TASK_ID" \
            --query 'ImportSnapshotTasks[0].SnapshotTaskDetail.SnapshotId' \
            --output text)"

    echo "SNAP=$SNAP" >> "$SHARED_ENVS"
    echo "Snapshot created: $SNAP"
}

# Registers an AMI based on an EBS snapshot defined in SNAP with a VOLUME_SIZE
# volume size.
register_ami() {
    echo "|----------------------AMI-----------------------|"
    AMI_ID=$(aws ec2 register-image \
        --name "$AMI_NAME" \
        --architecture x86_64 \
        --virtualization-type hvm \
        --ena-support \
        --boot-mode uefi \
        --imds-support v2.0 \
        --root-device-name /dev/xvda \
        --block-device-mappings \
            "DeviceName=/dev/xvda,Ebs={SnapshotId=$SNAP,VolumeSize=${VOLUME_SIZE:-35},VolumeType=gp3,DeleteOnTermination=true}" \
        --query ImageId --output text)
    
    echo "AMI_ID=$AMI_ID" >> "$SHARED_ENVS"
    echo "Waiting for AMI to become available..."
    aws ec2 wait image-available --image-ids "$AMI_ID"
    echo "AMI $AMI_NAME successfully registered: ${AMI_ID}"
}

# Wrapper command for the workflow around bootstrapping the AMI.
bootstrap_ami() {
    local CUSTOMIZED_IMG=""
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --customized-img)
                [[ $# -ge 2 ]] || { echo "--customized-img requires a value" >&2; exit 1; }
                CUSTOMIZED_IMG="$2"
                shift 2
                ;;
            *)
                echo "Unknown option: $1" >&2
                exit 1
                ;;
        esac
    done

    [[ -n "$CUSTOMIZED_IMG" && -f "$CUSTOMIZED_IMG" ]] || { echo "--customized-img must point to an existing file" >&2; exit 1; }

    local BUCKET="elemental-image-bucket"
    local PATH_IN_BUCKET="${PATH_IN_BUCKET:-"elemental/customized-${RUN_ID}.raw"}"
    local AMI_NAME="${AMI_NAME:-"elemental-ami-${RUN_ID}"}"

    echo "BUCKET_AND_PATH=$BUCKET/$PATH_IN_BUCKET" >> $SHARED_ENVS

    setup_s3_bucket
    setup_vmimport_role
    transfer_img_to_s3 "$CUSTOMIZED_IMG"
    import_snapshot
    register_ami
}

# Verifies that the --ignition-config-dir flag was provided and that the files inside it 
# match the number of desired nodes.
validate_ignition_configs() {
    [[ -n "$IGNITION_CONFIG_DIR" ]] || { echo "--ignition-config-dir is required" >&2; exit 1; }

    local IGNITION_CONFIG_COUNT=$(find "$IGNITION_CONFIG_DIR" -maxdepth 1 -type f | wc -l)
    [[ "$IGNITION_CONFIG_COUNT" -eq "$TOTAL_NODES" ]] || {
        echo "Expected $TOTAL_NODES ignition config files, found $IGNITION_CONFIG_COUNT" >&2
        exit 1
    }
}

# Sources the shared.env file that contains essential infrastructure IDs.
source_shared_envs() {
    [[ -f "$SHARED_ENVS" ]] || { echo "Shared env file '$SHARED_ENVS' does not exist" >&2; exit 1; }
    source "$SHARED_ENVS"
}

# Validates whether the shared.env file has a specific set of variables defined.
validate_shared_envs() {
    source_shared_envs

    local var
    for var in "$@"; do
        [[ -n "${!var:-}" ]] || { echo "Required variable '$var' is missing or empty in '$SHARED_ENVS'" >&2; exit 1; }
    done
}

# Launches a specific number of EC2 instances. Each instance is assigned a static IP generated during the bootstrap_infra command
# and propagated via shared.envs. Each instance is referring to an ignition configuration from the --ignition-config-dir defined
# directory location.
launch_ec2_instances() {
    echo "|------------------Launch EC2--------------------|"
    read -r -a NODE_IPS <<< "$NODE_IPS"
    [[ "${#NODE_IPS[@]}" -eq "$TOTAL_NODES" ]] || { echo "Expected $TOTAL_NODES node IPs, found ${#NODE_IPS[@]}" >&2; exit 1; }

    local IGNITION_CONFIGS=("$IGNITION_CONFIG_DIR"/*)

    INSTANCE_IDS=()
    for ((i=0; i<TOTAL_NODES; i++)); do
        echo "Launching EC2 instance. Type: $INSTANCE_TYPE. IP: ${NODE_IPS[$i]}. Ignition Config: ${IGNITION_CONFIGS[$i]}"
        ID="$(aws ec2 run-instances \
            --image-id "$AMI_ID" \
            --instance-type "$INSTANCE_TYPE" \
            --subnet-id "$SUBNET_ID" \
            --security-group-ids "$NODE_SG" \
            --private-ip-address "${NODE_IPS[$i]}" \
            --user-data "file://${IGNITION_CONFIGS[$i]}" \
            --associate-public-ip-address \
            --query 'Instances[0].InstanceId' \
            --output text)"
        INSTANCE_IDS+=("$ID")
        echo "INSTANCE_IDS=\"${INSTANCE_IDS[*]}\"" >> "$SHARED_ENVS"
    done

    echo "Waiting for instances to be running..."
    aws ec2 wait instance-running --instance-ids "${INSTANCE_IDS[@]}"
}

# Ensures that the given target has only health endpoints.
is_tg_healthy() {
    local description="$1"
    local tg="$2"
    local healthy unhealthy

    healthy="$(aws elbv2 describe-target-health \
        --target-group-arn "$tg" \
        --query 'TargetHealthDescriptions[?TargetHealth.State==`healthy`].Target.Id' \
        --output text)"

    unhealthy="$(aws elbv2 describe-target-health \
        --target-group-arn "$tg" \
        --query 'TargetHealthDescriptions[?TargetHealth.State!=`healthy`].Target.Id' \
        --output text)"

    printf "%-22s healthy[%s]  unhealthy[%s]\n" \
        "$description" "$healthy" "$unhealthy"

    [[ -z "$unhealthy" ]]
}

# Watches the target groups for the API, Supervisor, HTTP and HTTPS ingresses.
# Only after all endpoints are available can the cluster be considered ready.
watch_cluster_endpoints_health() {
    echo "Waiting for cluster endpoints to become healthy..."
    
    while true; do
        echo

        all_healthy=true
        is_tg_healthy "API (6443)"           "$API_TG"           || all_healthy=false
        is_tg_healthy "Supervisor (9345)"   "$SUPERVISOR_TG"    || all_healthy=false
        is_tg_healthy "Ingress HTTP (80)"   "$HTTP_INGRESS_TG"  || all_healthy=false
        is_tg_healthy "Ingress HTTPS (443)" "$HTTPS_INGRESS_TG" || all_healthy=false

        "$all_healthy" && break
        
        echo "Cluster endpoints not yet healthy. Retrying in 60 seconds..."
        sleep 60
    done

    echo
    echo "All cluster endpoints are healthy."
}

# Wrapper command for the workflow around spining up the actual UC cluster on AWS.
bootstrap_cluster() {
    local INSTANCE_TYPE="c7i-flex.large"
    local TOTAL_NODES=4
    local IGNITION_CONFIG_DIR=""

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --instance-type)
                [[ $# -ge 2 ]] || { echo "--instance-type requires a value" >&2; exit 1; }
                INSTANCE_TYPE="$2"
                shift 2
                ;;
            --ignition-config-dir)
                [[ $# -ge 2 ]] || { echo "--ignition-config-dir requires a value" >&2; exit 1; }
                [[ -d "$2" ]] || { echo "'$2' is not a directory" >&2; exit 1; }
                IGNITION_CONFIG_DIR="$2"
                shift 2
                ;;
            --nodes)
                [[ $# -ge 2 ]] || { echo "--nodes requires a value" >&2; exit 1; }
                [[ "$2" =~ ^[1-9][0-9]*$ ]] || { echo "--nodes value must be a positive integer" >&2; exit 1; }
                TOTAL_NODES="$2"
                shift 2
                ;;
            *)
                echo "Unknown option: $1" >&2
                exit 1
                ;;
        esac
    done

    validate_ignition_configs
    validate_shared_envs \
        AMI_ID SUBNET_ID NODE_SG NODE_IPS \
        API_TG SUPERVISOR_TG HTTP_INGRESS_TG HTTPS_INGRESS_TG
    launch_ec2_instances
    watch_cluster_endpoints_health
}

cleanup_instances() {
    [[ -n "${INSTANCE_IDS:-}" ]] && {
        if aws ec2 terminate-instances --instance-ids $INSTANCE_IDS; then
            echo "Waiting for termination of cluster EC2 instances: $INSTANCE_IDS..."

            aws ec2 wait instance-terminated --instance-ids $INSTANCE_IDS ||
                echo "Warning: failed waiting for EC2 instance termination: $INSTANCE_IDS" >&2
        else
            echo "Warning: failed to terminate EC2 instances: $INSTANCE_IDS" >&2
        fi
    }

    # Ensure func does not return 1 when variable check fails
    return 0
}

cleanup_lbs() {
    local lb_arn

    for lb_arn in "${INGRESS_LB_ARN:-}" "${CONTROL_LB_ARN:-}"; do
        [[ -n "$lb_arn" ]] || continue

        aws elbv2 delete-load-balancer --load-balancer-arn "$lb_arn" || {
            echo "Warning: failed to delete NLB: $lb_arn" >&2
            continue
        }
        echo "Waiting for NLB deletion. ARN: $lb_arn"
        aws elbv2 wait load-balancers-deleted --load-balancer-arns "$lb_arn" ||
            echo "Warning: timed out waiting for NLB deletion: $lb_arn" >&2
    done
}

cleanup_eip() {
    [[ -n "${INGRESS_EIP_ALLOC_ID:-}" ]] || return 0

    local i

    for ((i=1; i<=20; i++)); do
        aws ec2 release-address --allocation-id "$INGRESS_EIP_ALLOC_ID" 2>/dev/null && {
            echo "Released EIP: $INGRESS_EIP_ALLOC_ID"
            return 0
        }

        echo "EIP '$INGRESS_EIP_ALLOC_ID' could not be released, retrying ($i/20)..."
        sleep 10
    done

    echo "Warning: failed to release EIP '$INGRESS_EIP_ALLOC_ID' after 20 retries" >&2
    return 0
}

cleanup_tgs() {
    local tg_arn

    for tg_arn in "${HTTP_INGRESS_TG:-}" "${HTTPS_INGRESS_TG:-}" "${API_TG:-}" "${SUPERVISOR_TG:-}"; do
        [[ -n "$tg_arn" ]] || continue
        echo "Deleting target group: $tg_arn"
        aws elbv2 delete-target-group --target-group-arn "$tg_arn" || echo "Warning: failed to delete target group: $tg_arn" >&2
    done
}

cleanup_sgs() {
    local sg_id

    for sg_id in "${NODE_SG:-}" "${CONTROL_NLB_SG:-}" "${INGRESS_NLB_SG:-}"; do
        [[ -n "$sg_id" ]] || continue
        echo "Deleting security group: $sg_id"
        aws ec2 delete-security-group --group-id "$sg_id" || echo "Warning: failed to delete security group: $sg_id" >&2
    done
}

cleanup_infra() {
    cleanup_lbs
    cleanup_eip
    cleanup_tgs
    cleanup_sgs
}

cleanup_ami() {
    [[ -n "${AMI_ID:-}" ]] && {
        echo "Deregistering AMI: $AMI_ID"
        aws ec2 deregister-image --image-id "$AMI_ID" || echo "Warning: failed to deregister AMI: $AMI_ID" >&2
    }

    [[ -n "${SNAP:-}" ]] && {
        echo "Deleting snapshot: $SNAP"
        aws ec2 delete-snapshot  --snapshot-id "$SNAP" || echo "Warning: failed to delete snapshot: $SNAP" >&2
    }

    [[ -n "${BUCKET_AND_PATH:-}" ]] && {
        echo "Cleaning up S3: $BUCKET_AND_PATH"
        aws s3 rm "s3://${BUCKET_AND_PATH}" || echo "Warning: failed to clean up S3: $BUCKET_AND_PATH" >&2
    }

    # Ensure func does not return 1 when variable check fails
    return 0
}

# Wrapper for the clean up logic. It goes through all the IDs generated by previous commands
# in the shared.env file and attempts to clean up any non-empty ID.
teardown() {
    # No validation for vars is done here, so that the
    # command can handle partial teardown due to a failed
    # build-infra / build-ami / deploy-cluster command.
    source_shared_envs

    echo "|--------------------Teardown--------------------|"
    cleanup_instances
    cleanup_infra
    cleanup_ami
    
    echo "Removing shared envs file: $SHARED_ENVS"
    rm -f "$SHARED_ENVS"

    echo "Teardown complete."
}

usage() {
    cat <<EOF
Usage:
  $0 <command> [options]

Commands:
  bootstrap-infra
      Create the AWS infrastructure required for the UC cluster.

      Options:
        --allow-ssh                  Allow SSH access to the RKE2 nodes. Disabled by default.
        --static-cp-ips <count>      Number of static IPs reserved for control-plane nodes. Default: 3
        --static-wk-ips <count>      Number of static IPs reserved for worker nodes. Default: 1

  bootstrap-ami
      Upload a customized RAW image to S3, import it as an EBS snapshot,
      and register an EC2 AMI.

      Options:
        --customized-img <path>      Path to the customized RAW image. Required.

  bootstrap-cluster
      Launch EC2 instances using the previously created infrastructure
      and AMI.

      Options:
        --instance-type <type>       EC2 instance type. Default: c7i-flex.large
        --ignition-config-dir <path> Directory containing the Ignition configuration files. Required.
        --nodes <count>              Number of EC2 instances to launch. Default: 4

  teardown
      Teardown UC cluster and AWS infrastructure based on the shared.env file.

  help
      Show this help message.

Environment Variables:
  AWS_DEFAULT_REGION                 AWS region to use. Default: eu-central-1
  RUN_ID                             Identifier appended to generated AWS resource names.
  SHARED_ENVS                        File used to persist IDs and values between commands. Default: /<script-dir>/shared.env
EOF
}

run_with_teardown_on_failure() {
    trap '[[ -f "$SHARED_ENVS" ]] && teardown' EXIT
    "$@"
    trap - EXIT
}

main() {
    case "${1:-}" in
        bootstrap-infra)
            shift
            run_with_teardown_on_failure bootstrap_infra "$@"
            ;;
        bootstrap-ami)
            shift
            run_with_teardown_on_failure bootstrap_ami "$@"
            ;;
        bootstrap-cluster)
            shift
            run_with_teardown_on_failure bootstrap_cluster "$@"
            ;;
        teardown)
            shift
            teardown
            ;;
        help|-h|--help)
            usage
            ;;
        *)
            [[ -n "${1:-}" ]] && echo "Unknown command: $1" >&2
            usage >&2
            exit 1
            ;;
    esac
}

main "$@"
