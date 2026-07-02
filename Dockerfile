FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/azure-devops-exporter .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/azure-devops-exporter /azure-devops-exporter
# Numeric UID:GID, not the "nonroot" name — Kubernetes' runAsNonRoot check (used by this
# chart's securityContext) can only verify a non-root user from the image if it's numeric;
# a symbolic USER name makes it fail with CreateContainerConfigError even though 65532 (this
# base image's actual nonroot UID/GID) is itself non-root.
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/azure-devops-exporter"]
