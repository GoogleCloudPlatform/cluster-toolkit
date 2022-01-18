.PHONY: tests fmt vet test-engine test-resources test-examples packer \
        packer-clean packer-check packer-docs add-google-license, \
        check-tflint, check-pre-commit, install-deps-dev, ghpc-dev, \
        check-terraform-exists, check-packer-exists, \
				check-terraform-version, check-packer-version
RES = ./resources
ENG = ./cmd/... ./pkg/...
SRC = $(ENG) $(RES)/tests/...
TF_VERSION_CHECK=$(shell expr `terraform version | head -n1 | cut -f 2- -d ' ' | cut -c 2-` \>= 0.14)
PACKER_FOLDERS=$(shell find ${RES} -type f -name "*.pkr.hcl" -not -path '*/\.*' -printf '%h\n' | sort -u)

ghpc: check-terraform-exists check-terraform-version $(shell find ./cmd ./pkg ./resources ghpc.go -type f)
	$(info **************** building ghpc ************************)
	go build ghpc.go

ghpc-dev: fmt vet ghpc

tests: vet packer-check test-engine test-resources test-examples

fmt:
	$(info **************** formatting go code *******************)
	go fmt $(SRC)

vet:
	$(info **************** vetting go code **********************)
	go vet $(SRC)

test-engine:
	$(info **************** running ghpc unit tests **************)
	go test -cover $(ENG) 2>&1 |  perl tools/enforce_coverage.pl

test-resources:
	$(info **************** running resources unit tests *********)
	go test $(RES)/...

test-examples: ghpc-dev
	$(info *********** running basic integration tests ***********)
	tools/test_examples/test_examples.sh

packer: check-packer-exists check-packer-version packer-clean packer-docs

packer-clean: check-packer-exists check-packer-version
	$(info **************** formatting packer files **************)
	@for folder in ${PACKER_FOLDERS}; do \
	  echo "cleaning syntax for $${folder}";\
		packer fmt $${folder};\
	done

packer-check: check-packer-exists check-packer-version
	$(info **************** checking packer syntax ***************)
	@for folder in ${PACKER_FOLDERS}; do \
	  echo "checking syntax for $${folder}"; \
	  packer fmt -check $${folder}; \
	done

ifeq (, $(shell which terraform))
check-terraform-exists:
	$(error ERROR: terraform not installed, visit https://learn.hashicorp.com/tutorials/terraform/install-cli)
else
check-terraform-exists:

endif

ifneq ("$(TF_VERSION_CHECK)", "1")
check-terraform-version:
	$(error ERROR: terraform version must be greater than 0.14, update at https://learn.hashicorp.com/tutorials/terraform/install-cli)
else
check-terraform-version:

endif

ifeq (, $(shell which packer))
check-packer-exists:
	$(error ERROR: packer not installed, visit https://learn.hashicorp.com/tutorials/packer/get-started-install-cli)
else
PK_VERSION_CHECK=$(shell expr `packer version | head -n1 | cut -f 2- -d ' ' | cut -c 2-` \>= 1.6)
check-packer-exists:
endif

ifneq ("$(PK_VERSION_CHECK)", "1")
check-packer-version:
	$(error ERROR: packer version must be greater than 1.6.6, update at https://learn.hashicorp.com/tutorials/packer/get-started-install-cli)
else
check-packer-version:

endif

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

install-deps-dev: check-pre-commit check-tflint
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
