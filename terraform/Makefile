SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.NOTPARALLEL:

PROVIDER      ?= aws
PROFILE       ?= multi-node
ID            ?= $(PROVIDER)-$(PROFILE)
PLATFORM      ?= linux/amd64
REGION        ?=
MACHINE_TYPE  ?=
SSH_CIDR      ?=
ELEMENTAL_IMAGE ?=

DIR   := $(realpath $(dir $(lastword $(MAKEFILE_LIST))))
STACK := $(DIR)/providers/$(PROVIDER)

WORK_DIR      := $(DIR)/.work/$(ID)
STATE_DIR     := $(DIR)/.state/$(ID)
INFRA_STATE   := $(STATE_DIR)/infra.tfstate
CLUSTER_STATE := $(STATE_DIR)/cluster.tfstate

# Path to local elemental3 binary, fallback if ELEMENTAL_IMAGE is not provided.
ELEMENTAL_BIN ?= $(abspath $(DIR)/..)/build/elemental3

export ELEMENTAL_IMAGE ELEMENTAL_BIN PLATFORM

# terraform - needed to spin up the cloud resources
# jq        - needed to parse terraform's outputs
# butane    - needed to produce ignition configurations from the runtime examples
# podman    - needed to produce the customized image from the config examples,
#             only when running customization from a container image
REQ_TOOLS = terraform jq butane
ifneq (,$(ELEMENTAL_IMAGE))
REQ_TOOLS += podman
endif

INFRA_VARS   = -var id=$(ID) \
               -var profile=$(PROFILE) \
               $(if $(REGION),-var region=$(REGION),) \
               $(if $(SSH_CIDR),-var ssh_cidr=$(SSH_CIDR),)

CLUSTER_VARS = -var id=$(ID) \
               -var infra_state_path=$(INFRA_STATE) \
               -var customized_img_path=$(WORK_DIR)/config/customized.raw \
               -var ignition_dir=$(WORK_DIR)/ignition \
               -var platform=$(PLATFORM) \
               $(if $(REGION),-var region=$(REGION),) \
               $(if $(MACHINE_TYPE),-var machine_type=$(MACHINE_TYPE),)

TF_INFRA   = terraform -chdir=$(STACK)/infra
TF_CLUSTER = terraform -chdir=$(STACK)/cluster

# Each id keeps its state under .state/<id>, so both stacks are pointed at a state
# file rather than dropping one inside the stack directory.
TF_INFRA_INIT   = $(TF_INFRA) init -input=false -reconfigure -backend-config=path=$(INFRA_STATE)
TF_CLUSTER_INIT = $(TF_CLUSTER) init -input=false -reconfigure -backend-config=path=$(CLUSTER_STATE)

define HELP_TEXT
Targets:
  up                Spins a single/multi node Elemental cluster on the specified provider.
                    The image used to spin the cluster is produced from the
                    'examples/elemental/customize/$(PROVIDER)' configuration directory.

  destroy           Destroys all $(PROVIDER) resources created by 'make up'.

Supported variables:
  PROVIDER          Provider on which to deploy the Elemental cluster.
                    Default: aws. Supported values: [aws].

  PROFILE           Whether to deploy a single-node or multi-node cluster.
                    Default: multi-node. Supported values: [single-node, multi-node].

  ID                Identifier all created resources are named after. Up to 20 lowercase
                    alphanumeric or dash characters, not starting or ending with a dash.
                    Default: '<provider>-<profile>'. Must be passed explicitly if a cluster
                    with this ID already exists on your provider region.

  REGION            Region in the provider where the cluster will be deployed.
                    Default: depends on the provider; eu-central-1 for aws.

  MACHINE_TYPE      Type for the machines that will be hosting the cluster.
                    Default: depends on the provider; c7i-flex.large for aws.

  PLATFORM          Platform the image is customized for, as <os>/<arch>.
                    Must match the supported architecture of the MACHINE_TYPE.
                    Default: linux/amd64. Supported values: [linux/amd64, linux/arm64].

  SSH_CIDR          Source IP range allowed to reach the nodes on port 22.
                    Default: empty, leaves SSH closed.

  ELEMENTAL_IMAGE   Elemental image to use for the image customization process.
                    If empty, the local binary will be used.
endef
export HELP_TEXT

.DEFAULT_GOAL := help
.PHONY: help
help:
	@printf '%s\n' "$$HELP_TEXT"

.PHONY: up
up: validate-tools infra customize cluster

# Validate that the necessary tools for a successful operation are present.
# Runs before anything is created, so that a missing prerequisite does not
# surface only after the provider resources are already up.
.PHONY: validate-tools
validate-tools:
	@for t in $(REQ_TOOLS); do \
	    command -v $$t >/dev/null || { echo "error: '$$t' is not installed" >&2; exit 1; }; \
	done
	@[[ -n "$(ELEMENTAL_IMAGE)" || -x "$(ELEMENTAL_BIN)" ]] || \
	    { echo "error: set ELEMENTAL_IMAGE or build $(ELEMENTAL_BIN) with 'make -C $(abspath $(DIR)/..)'" >&2; exit 1; }

# Spin necessary infrastructure resources so that an image can be customized and
# hold the correct values.
.PHONY: infra
infra:
	@mkdir -p $(WORK_DIR) $(STATE_DIR)
	@echo "Using cluster id '$(ID)'"
	$(TF_INFRA_INIT)
	$(TF_INFRA) apply -input=false -auto-approve $(INFRA_VARS)
	$(TF_INFRA) output -json > $(WORK_DIR)/infra.json

# Customize an image using the 'examples/elemental/customize/<provider>' that matches
# the PROVIDER variable.
.PHONY: customize
customize:
	@$(DIR)/customize.sh $(PROVIDER) $(PROFILE) $(WORK_DIR)

# Create a cluster based on the state of the infra tf module. Must be run after 'make infra'.
# Note: Failures due to missing infra tf state will be graciously handled during the plan step of 'terraform apply'.
.PHONY: cluster
cluster:
	$(TF_CLUSTER_INIT)
	$(TF_CLUSTER) apply -input=false -auto-approve $(CLUSTER_VARS)

# Returns the output variables of the cluster tf module, showcasing what was created.
# Useful to retrieve the outputs for a specific ID.
.PHONY: outputs
outputs:
	@$(TF_CLUSTER_INIT) >/dev/null
	@$(TF_CLUSTER) output

# Destroys the created resources from bottom up (cluster -> infra -> build dir).
# If an error occurs, stops immediately, preserving the later resources. This allows
# for an investigation of the failure and a possible follow-up 'make destroy'.
.PHONY: destroy
destroy:
	@echo "Destroying cluster id '$(ID)'"
	$(TF_CLUSTER_INIT)
	$(TF_CLUSTER) destroy -input=false -auto-approve $(CLUSTER_VARS)
	$(TF_INFRA_INIT)
	$(TF_INFRA) destroy -input=false -auto-approve $(INFRA_VARS)
	@rm -rf $(WORK_DIR) $(STATE_DIR)
