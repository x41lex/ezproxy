FROM golang:1.22.2 as build

WORKDIR /go/src/ezproxy 
ADD . .
RUN go mod download 
# RUN go vet -v 
# RUN go test -v

RUN CGO_ENABLED=0 go build -o /go/bin/ezproxy

FROM gcr.io/distroless/static

EXPOSE 8080
EXPOSE 5554
ADD config.yaml /
COPY --from=build /go/bin/ezproxy /
CMD ["/ezproxy"]