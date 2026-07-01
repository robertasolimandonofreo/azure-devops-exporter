FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/azure-devops-exporter .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/azure-devops-exporter /azure-devops-exporter
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/azure-devops-exporter"]
