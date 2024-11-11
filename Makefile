IMG ?= rockswe/K8s-PodConfigMapController:latest

all: manager

test: fmt vet
    go test ./... -coverprofile cover.out

manager:
    go build -o bin/manager main.go

run: fmt vet
    go run ./main.go

install: manifests
    kubectl apply -f config/crd/bases

uninstall:
    kubectl delete -f config/crd/bases

deploy: manifests
    kustomize build config/default | kubectl apply -f -

manifests:
    controller-gen rbac:roleName=podconfigmapcontroller-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

fmt:
    go fmt ./...

vet:
    go vet ./...

docker-build:
    docker build -t ${IMG} .

docker-push:
    docker push ${IMG}
