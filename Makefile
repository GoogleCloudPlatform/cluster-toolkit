.PHONY: tests fmt vet test-engine test-resources test-examples packer packer-clean packer-check packer-docs
RES = ./resources
ENG = ./cmd/... ./pkg/...
SRC = $(ENG) $(RES)/tests/...
PACKER_FOLDERS=$(shell find ${RES} -type f -name "*.pkr.hcl" -not -path '*/\.*' -printf '%h\n' | sort -u)

ghpc: fmt vet
	$(info **************** building ghpc ************************)
	go build ghpc.go

tests: vet packer-check test-engine test-resources test-examples

fmt:
	$(info **************** formatting go code *******************)
	go fmt $(SRC)

vet:
	$(info **************** vetting go code **********************)
	go vet $(SRC)

test-engine:
	$(info **************** running ghpc unit tests **************)
	go test -cover $(ENG)

test-resources:
	$(info **************** running resources unit tests *********)
	go test $(RES)/...

test-examples: ghpc
	$(info **************** running basic integration tests ******)
	tools/test_examples/test_examples.sh

packer: packer-clean packer-docs

packer-clean:
	$(info **************** formatting packer files **************)
	@for folder in ${PACKER_FOLDERS}; do \
	  echo "cleaning syntax for $${folder}";\
		packer fmt $${folder};\
	done

packer-check:
	$(info **************** checking packer syntax ***************)
	@for folder in ${PACKER_FOLDERS}; do \
	  echo "checking syntax for $${folder}"; \
	  packer fmt -check $${folder}; \
	done

ifeq (, $(shell which terraform-docs))
packer-docs:
	$(error "could not find terraform-docs in PATH, run: go install github.com/terraform-docs/terraform-docs@v0.16.0")
else
packer-docs:
	$(info **************** creating packer documentation ********)
	@for folder in ${PACKER_FOLDERS}; do \
	  echo "creating documentation for $${folder}";\
		terraform-docs markdown $${folder} --config .tfdocs-markdown.yaml;\
		terraform-docs json $${folder} --config .tfdocs-json.yaml;\
	done
endif
