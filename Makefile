include .env
export
export CGO_ENABLED=0
export GOOS=linux

GO := vgo
BUILD := $(GO) build -a -ldflags '-extldflags "-static"'

ifeq ($(shell git describe --always > /dev/null 2>&1 ; echo $$?), 0)
	VERSION := :$(shell git describe --always --dirty=-git)
endif
ifeq ($(shell git describe --tags > /dev/null 2>&1 ; echo $$?), 0)
	VERSION := :$(shell git describe --tags)
endif

app: main.go $(wildcard ./app/*.go) $(wildcard ./api/*.go) $(wildcard ./models/*.go)
	$(BUILD) -o bin/$@ ./main.go

bootstrap: cli/bootstrap/main.go
	$(BUILD) -o bin/$@ cli/bootstrap/main.go

votes: cli/votes/main.go
	$(BUILD) -o bin/$@ cli/votes/main.go

cli: bootstrap votes

clean:
	$(RM) bin/*

run: app
	@./bin/app

image: app
	cp -r bin docker/
	cp -r templates docker/
	cp -r assets docker/
	cd docker && $(MAKE) $@

compose: app
	cp -r bin docker/
	cp -r templates docker/
	cp -r assets docker/
	cd docker && $(MAKE) $@
