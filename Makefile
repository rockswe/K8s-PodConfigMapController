IMG ?= rockswe/k8s-podconfigmapcontroller:latest

# Target for building the controller binary
CONTROLLER_BIN := bin/controller

all: $(CONTROLLER_BIN)

$(CONTROLLER_BIN): fmt vet
	go build -o $(CONTROLLER_BIN) main.go

# Run tests
test: fmt vet
	go test ./... -coverprofile cover.out

# Run the controller locally
run: fmt vet
	go run ./main.go

# Generate deepcopy, client, lister, informer typically
# For now, just deepcopy as we are using dynamic client for PCMC
generate: # Ensure controller-gen is installed and in PATH or use $(GOPATH)/bin/controller-gen
	@echo "Ensuring $(GOPATH)/bin is in your PATH or controller-gen is installed globally."
	$(GOPATH)/bin/controller-gen object paths="./api/v1alpha1/..." output:dir=./api/v1alpha1

# Generate CRD manifests
manifests:
	@echo "Ensuring $(GOPATH)/bin is in your PATH or controller-gen is installed globally."
	$(GOPATH)/bin/controller-gen crd paths="./api/v1alpha1/..." output:crd:dir=./crd

# Install CRD into the cluster
install-crd:
	kubectl apply -f crd/podconfigmapconfig_crd.yaml

# Uninstall CRD from the cluster
uninstall-crd:
	kubectl delete -f crd/podconfigmapconfig_crd.yaml --ignore-not-found=true

# Deploy controller to the cluster using manifests
deploy:
	kubectl apply -f manifests/rbac.yaml
	kubectl apply -f manifests/deployment.yaml

# Undeploy controller from the cluster
undeploy:
	kubectl delete -f manifests/deployment.yaml --ignore-not-found=true
	kubectl delete -f manifests/rbac.yaml --ignore-not-found=true

# Format go code
fmt:
	go fmt ./...

# Run go vet
vet:
	go vet ./...

# Build the docker image
docker-build:
	docker build -t ${IMG} .

# Push the docker image
docker-push:
	docker push ${IMG}

.PHONY: all test run generate manifests install-crd uninstall-crd deploy undeploy fmt vet docker-build docker-push
