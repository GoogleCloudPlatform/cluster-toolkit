# PREAMBLE
MIN_PACKER_VERSION=1.7.9 # for building images
MIN_TERRAFORM_VERSION=1.2 # for deploying modules
MIN_GOLANG_VERSION=1.18 # for building ghpc

.PHONY: install install-user tests format add-google-license install-dev-deps \
        warn-go-missing warn-terraform-missing warn-packer-missing \
        warn-go-version warn-terraform-version warn-packer-version \
        test-engine validate_configs validate_golden_copy packer-check \
        terraform-format packer-format \
        check-tflint check-pre-commit

SHELL=/bin/bash -o pipefail
ENG = ./cmd/... ./pkg/...
TERRAFORM_FOLDERS=$(shell find ./modules ./community/modules ./tools -type f -name "*.tf" -not -path '*/\.*' -exec dirname "{}" \; | sort -u)
PACKER_FOLDERS=$(shell find ./modules ./community/modules ./tools -type f -name "*.pkr.hcl" -not -path '*/\.*' -exec dirname "{}" \; | sort -u)

ifneq (, $(shell which git))
## GIT IS PRESENT
ifneq (,$(wildcard .git))
## GIT DIRECTORY EXISTS
GIT_TAG_VERSION=$(shell git tag --points-at HEAD)
GIT_BRANCH=$(shell git branch --show-current)
GIT_COMMIT_INFO=$(shell git describe --tags --dirty --long --always)
GIT_COMMIT_HASH=$(shell git rev-parse HEAD)
GIT_INITIAL_HASH=$(shell git rev-list --max-parents=0 HEAD)
endif
endif

# RULES MEANT TO BE USED DIRECTLY

ghpc: warn-go-version warn-terraform-version warn-packer-version $(shell find ./cmd ./pkg ghpc.go -type f)
	$(info **************** building ghpc ************************)
	@go build -ldflags="-X 'main.gitTagVersion=$(GIT_TAG_VERSION)' -X 'main.gitBranch=$(GIT_BRANCH)' -X 'main.gitCommitInfo=$(GIT_COMMIT_INFO)' -X 'main.gitCommitHash=$(GIT_COMMIT_HASH)' -X 'main.gitInitialHash=$(GIT_INITIAL_HASH)'" ghpc.go

install-user:
	$(info ******** installing ghpc in ~/bin *********************)
	mkdir -p ~/bin
	install ./ghpc ~/bin

ifeq ($(shell id -u), 0)
install:
	$(info ***** installing ghpc in /usr/local/bin ***************)
	install ./ghpc /usr/local/bin

else
install: install-user

endif

tests: warn-terraform-version warn-packer-version test-engine validate_golden_copy validate_configs packer-check

format: warn-go-version warn-terraform-version warn-packer-version terraform-format packer-format
	$(info **************** formatting go code *******************)
	go fmt $(ENG)

install-dev-deps: warn-terraform-version warn-packer-version check-pre-commit check-tflint check-shellcheck
	$(info *********** installing developer dependencies *********)
	go install github.com/terraform-docs/terraform-docs@latest
	go install golang.org/x/lint/golint@latest
	go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
	go install github.com/go-critic/go-critic/cmd/gocritic@latest
	go install github.com/google/addlicense@latest
	go install mvdan.cc/sh/v3/cmd/shfmt@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install honnef.co/go/tools/cmd/staticcheck@latest

ifeq (, $(shell which addlicense))
add-google-license:
	$(error "could not find addlicense in PATH, run: go install github.com/google/addlicense@latest")
else
add-google-license:
	# lysozyme-example is under CC-BY-4.0
	addlicense -c "Google LLC" -l apache -ignore **/lysozyme-example/submit.sh .
endif

# RULES SUPPORTING THE ABOVE

test-engine: warn-go-missing
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

ifeq (, $(shell which shellcheck))
check-shellcheck:
	$(info WARNING: shellcheck not installed, visit https://github.com/koalaman/shellcheck#installing for installation instructions.)
else
check-shellcheck:

endif

###################################
# GO SECTION
ifeq (, $(shell which go))
## GO IS NOT PRESENT
warn-go-missing:
	$(error ERROR: could not find go in PATH, visit: https://go.dev/doc/install)

warn-go-version: warn-go-missing

else
## GO IS PRESENT
warn-go-missing:

GO_VERSION=$(shell go version | cut -f 3 -d ' ')
GO_VERSION_CHECK=$(shell ./tools/version_check.sh $(GO_VERSION) $(MIN_GOLANG_VERSION))
ifneq ("$(GO_VERSION_CHECK)", "yes")
warn-go-version:
	$(warning WARNING: Go version must be greater than $(MIN_GOLANG_VERSION), update at  https://go.dev/doc/install)
else
warn-go-version:

endif
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

TF_VERSION=$(shell terraform version | cut -f 2- -d ' ' | head -n1)
TF_VERSION_CHECK=$(shell ./tools/version_check.sh $(TF_VERSION) $(MIN_TERRAFORM_VERSION))
ifneq ("$(TF_VERSION_CHECK)", "yes")
warn-terraform-version:
	$(warning WARNING: terraform version must be greater than $(MIN_TERRAFORM_VERSION), update at https://learn.hashicorp.com/tutorials/terraform/install-cli)
else
warn-terraform-version:
endif

validate_configs: ghpc
	$(info *********** running basic integration tests ***********)
	tools/validate_configs/validate_configs.sh

validate_golden_copy: ghpc
	$(info *********** running "Golden copy" tests ***********)
	tools/validate_configs/golden_copies/validate.sh

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
ifneq (yes, $(shell  ./tools/detect_packer.sh ))
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

PK_VERSION=$(shell packer version | cut -f 2- -d ' ' | head -n1)
PK_VERSION_CHECK=$(shell ./tools/version_check.sh $(PK_VERSION) $(MIN_PACKER_VERSION))
ifneq ("$(PK_VERSION_CHECK)", "yes")
### WRONG PACKER VERSION, MAY ALSO MEAN THE USER HAS SOME OTHER PACKER TOOL
warn-packer-version:
	$(warning WARNING: packer version must be greater than $(MIN_PACKER_VERSION), update at https://learn.hashicorp.com/tutorials/packer/get-started-install-cli)

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
