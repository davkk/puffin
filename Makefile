.PHONY: mail run seed dev run-dovecot test clean

DOVECOT_IMAGE ?= dovecot/dovecot:latest-dev
DOVECOT_PORT  ?= 31143

run:
	go run main.go

seed:
	./seed.sh

dev: clean run-dovecot seed
	@find . -name '*.go' | entr -r go run main.go

run-dovecot:
	@if ! docker ps --format '{{.Names}}' | grep -q '^dovecot-test$$'; then \
		echo "Starting Dovecot..."; \
		docker run -d --name dovecot-test \
			-p 143:$(DOVECOT_PORT) \
			-e USER_PASSWORD=password \
			$(DOVECOT_IMAGE); \
		sleep 2; \
	fi

mail:
	docker run -d --name dovecot-test \
		-p 143:$(DOVECOT_PORT) \
		-e USER_PASSWORD=password \
		$(DOVECOT_IMAGE)
	make seed

test:
	go test -v -count=1 ./pkg/...

clean:
	docker rm -f dovecot-test 2>/dev/null
	rm puffin.db* 2>/dev/null
