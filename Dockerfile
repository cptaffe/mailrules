FROM golang:1.21 AS build

WORKDIR /usr/src/mailrules

RUN go install modernc.org/goyacc@latest

# pre-copy/cache go.mod for pre-downloading dependencies and only redownloading them in subsequent builds if they change
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY parse ./parse
RUN go generate ./parse
COPY rules ./rules
COPY main.go .
RUN go build -v -o /usr/local/bin/mailrules .

ENTRYPOINT ["/usr/local/bin/mailrules"]
