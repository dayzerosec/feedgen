build:
	go build -o feedgen feedgen/cmd/feedgen

clean:
	del feedgen

genrss:
	go run feedgen -f