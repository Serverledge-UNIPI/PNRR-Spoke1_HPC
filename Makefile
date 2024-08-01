BIN=bin
CFLAGS=-shared -fPIC -I/usr/include/python3.10
LDFLAGS=-lpython3.10

all: serverledge executor serverledge-cli lb

serverledge: $(BIN)/libpysolver.so
	CGO_ENABLED=1 GOOS=linux go build -o $(BIN)/$@ cmd/$@/main.go

lb:
	CGO_ENABLED=0 GOOS=linux go build -o $(BIN)/$@ cmd/$@/main.go

serverledge-cli:
	CGO_ENABLED=1 GOOS=linux go build -o $(BIN)/$@ cmd/cli/main.go

executor:
	CGO_ENABLED=0 GOOS=linux go build -o $(BIN)/$@ cmd/$@/executor.go

$(BIN)/libpysolver.so: internal/solver/pysolver.c | $(BIN)
	gcc $(CFLAGS) $(LDFLAGS) -o $@ $<

$(BIN):
	mkdir -p $(BIN)

clean:
	rm -rf $(BIN)

DOCKERHUB_USER=grussorusso
images:  image-python310 image-nodejs17ng image-base
image-python310:
	docker build -t $(DOCKERHUB_USER)/serverledge-python310 -f images/python310/Dockerfile .
image-base:
	docker build -t $(DOCKERHUB_USER)/serverledge-base -f images/base-alpine/Dockerfile .
image-nodejs17ng:
	docker build -t $(DOCKERHUB_USER)/serverledge-nodejs17ng -f images/nodejs17ng/Dockerfile .

push-images:
	docker push $(DOCKERHUB_USER)/serverledge-python310
	docker push $(DOCKERHUB_USER)/serverledge-base
	docker push $(DOCKERHUB_USER)/serverledge-nodejs17ng

test:
	go test -v ./...

.PHONY: serverledge serverledge-cli lb executor test images
