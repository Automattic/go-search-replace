FROM golang:1.26-alpine3.22@sha256:be93003ee861b3b91b6ebcb22678524947e0cd786c2df3f32af520006b1e54f5 AS build

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /usr/bin/go-search-replace .

FROM scratch

COPY --from=build /usr/bin/go-search-replace /usr/bin/go-search-replace

USER 65532:65532

ENTRYPOINT [ "/usr/bin/go-search-replace" ]
