# PREAMBLE
.PHONY: tests format add-google-license install-dev-deps \
        warn-terraform-missing warn-packer-missing \
				warn-terraform-version warn-packer-version \
				test-engine validate_configs packer-check \
				terraform-format packer-format \
        check-tflint check-pre-commit

ENG = ./cmd/... ./pkg/...
TERRAFORM_FOLDERS=$(shell find ./resources ./tools -type f -name "*.tf" -not -path '*/\.*' -printf '%h\n' | sort -u)
PACKER_FOLDERS=$(shell find ./resources ./tools -type f -name "*.pkr.hcl" -not -path '*/\.*' -printf '%h\n' | sort -u)

# RULES MEANT TO BE USED DIRECTLY

ghpc: warn-terraform-version warn-packer-version $(shell find ./cmd ./pkg ghpc.go -type f)
	$(info **************** building ghpc ************************)
	go build ghpc.go

tests: warn-terraform-version warn-packer-version test-engine validate_configs packer-check

format: warn-terraform-version warn-packer-version terraform-format packer-format
	$(info **************** formatting go code *******************)
	go fmt $(ENG)

install-dev-deps: warn-terraform-version warn-packer-version check-pre-commit check-tflint
	$(info *********** installing developer dependencies *********)
	go install github.com/terraform-docs/terraform-docs@latest
	go install golang.org/x/lint/golint@latest
	go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
	go install github.com/go-critic/go-critic/cmd/gocritic@latest
	go install github.com/google/addlicense@latest

ifeq (, $(shell which addlicense))
add-google-license:
	$(error "could not find addlicense in PATH, run: go install github.com/google/addlicense@latest")
else
add-google-license:
	addlicense -c "Google LLC" -l apache .
endif

# RULES SUPPORTING THE ABOVE

test-engine:
	$(info **************** vetting go code **********************)
	go vet $(ENG)
	$(info **************** running ghpc unit tests **************)
	go test -cover $(ENG) 2>&1 |  perl tools/enforce_coverage.pl

ifeq (, $(shell which pre-commit))
check-pre-commit:
	$(info WARNING: pre-commit not installed, visit https://pre-commit.com/ for installation instructions.)
else
check-pre-commit:

endif

ifeq (, $(shell which tflint))
check-tflint:
	$(info WARNING: tflint not installed, visit https://github.com/terraform-linters/tflint#installation for installation instructions.)
else
check-tflint:

endif

###################################
# TERRAFORM SECTION
ifeq (, $(shell which terraform))
## TERRAFORM IS NOT PRESENT
warn-terraform-missing:
	$(warning WARNING: terraform not installed and deployments will not work in this machine, visit https://learn.hashicorp.com/tutorials/terraform/install-cli)

warn-terraform-version: warn-terraform-missing

terraform-format:
	$(warning WARNING: not formatting terraform)

validate_configs:
	$(error ERROR: validate configs requires terraform)

else
## TERRAFORM IS PRESENT
warn-terraform-missing:

TF_VERSION_CHECK=$(shell expr `terraform version | cut -f 2- -d ' ' | cut -c 2- | head -n1` \>= 1.0)
ifneq ("$(TF_VERSION_CHECK)", "1")
warn-terraform-version:
	$(warning WARNING: terraform version must be greater than 1.0.0, update at https://learn.hashicorp.com/tutorials/terraform/install-cli)
else
warn-terraform-version:
endif

validate_configs: ghpc
	$(info *********** running basic integration tests ***********)
	tools/validate_configs/validate_configs.sh

terraform-format:
	$(info *********** cleaning terraform files syntax and generating terraform documentation ***********)
	@for folder in ${TERRAFORM_FOLDERS}; do \
	  echo "cleaning syntax for $${folder}";\
		terraform fmt -list=true $${folder};\
	done
	@for folder in ${TERRAFORM_FOLDERS}; do \
		terraform-docs markdown $${folder} --config .tfdocs-markdown.yaml;\
		terraform-docs json $${folder} --config .tfdocs-json.yaml;\
	done

endif
# END OF TERRAFORM SECTION
###################################
# PACKER SECTION
ifeq (, $(shell which packer))
## PACKER IS NOT PRESENT
warn-packer-missing:
	$(warning WARNING: packer not installed, visit https://learn.hashicorp.com/tutorials/packer/get-started-install-cli)

warn-packer-version: warn-packer-missing

packer-check: warn-packer-missing
	$(warning WARNING: packer not installed, not checking packer code)

packer-format: warn-packer-missing
	$(warning WARNING: packer not installed, not formatting packer code)

else
## PACKER IS PRESENT
warn-packer-missing:

PK_VERSION_CHECK=$(shell expr `packer version | cut -f 2- -d ' ' | cut -c 2- | head -n1` \>= 1.6)
ifneq ("$(PK_VERSION_CHECK)", "1")
### WRONG PACKER VERSION, MAY ALSO MEAN THE USER HAS SOME OTHER PACKER TOOL
warn-packer-version:
	$(warning WARNING: packer version must be greater than 1.6.6, update at https://learn.hashicorp.com/tutorials/packer/get-started-install-cli)

packer-check: warn-packer-version
	$(warning WARNING: wrong packer version, not checking packer code)

packer-format: warn-packer-version
	$(warning WARNING: wrong packer version, not formatting packer code)

else
### PACKER INSTALLED WITH THE RIGHT VERSION
warn-packer-version:

packer-check:
	$(info **************** checking packer syntax ***************)
	@for folder in ${PACKER_FOLDERS}; do \
	  echo "checking syntax for $${folder}"; \
	  packer fmt -check $${folder}; \
	done

packer-format:
	$(info **************** formatting packer files and generating packer documentation **************)
	@for folder in ${PACKER_FOLDERS}; do \
	  echo -e "cleaning syntax for $${folder}\n";\
		packer fmt $${folder};\
	done
	@for folder in ${PACKER_FOLDERS}; do \
		terraform-docs markdown $${folder} --config .tfdocs-markdown.yaml;\
		terraform-docs json $${folder} --config .tfdocs-json.yaml;\
	done

endif
endif
# END OF PACKER SECTION

