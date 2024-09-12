# Makefile

# Define the default target when no target is provided
.PHONY: all
all: build

# The target to build the Go application
build:
	go build -o cosi main.go

# The deploy target that takes a variable for the host
# Usage: make deploy HOST=<hostname_or_ip>
deploy: build
	@if [ -z "$(HOST)" ]; then \
		echo "Error: HOST variable is required"; \
		exit 1; \
	fi
	ssh ubuntu@$(HOST) killall cosi && sudo rm cosi || true
	scp cosi ubuntu@$(HOST):cosi
	ssh ubuntu@$(HOST) PORT=80 sudo ./cosi

# Clean target to remove built files
clean:
	rm -f cosi
