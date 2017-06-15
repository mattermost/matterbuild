build:
	@echo Building

	rm -rf dist/
	mkdir -p dist/matterbuild
	go build
	mv matterbuild dist/matterbuild/
	cp config.json dist/matterbuild/


package: build
	tar -C dist -czf dist/matterbuild.tar.gz matterbuild
