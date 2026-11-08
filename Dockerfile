# HCL DX Script Application operator.
# The runtime image is HCL's public DXClient image (hclcr.io/dx-public/dxclient) plus the operator binary: the
# operator runs DXClient itself to push Script Applications, so no Job or extra image is needed.
# DXClient is HCL software under its own licence (see LICENSE-DXClient.md): the operator does not run it unless
# the licence is accepted (DXCLIENT_ACCEPT_LICENSE=true, Helm value dxclient.acceptLicense).
ARG DXCLIENT_IMAGE=hclcr.io/dx-public/dxclient:v95_CF239_20261103-1404

FROM golang:1.26 AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -trimpath -ldflags="-s -w" -o manager ./cmd

FROM ${DXCLIENT_IMAGE}
LABEL org.opencontainers.image.title="HCL DX Script Application operator" \
      org.opencontainers.image.source="https://github.com/europesip/hcl-dx-scriptapp-operator" \
      org.opencontainers.image.licenses="MIT AND LicenseRef-HCL-DXClient" \
      org.opencontainers.image.description="Kubernetes operator that keeps HCL DX Script Applications, pages and DAM collections as declared (includes HCL DXClient)"
COPY --from=builder /workspace/manager /usr/local/bin/manager
COPY LICENSE LICENSE-DXClient.md /licenses/
ENV DXCLIENT_HOME=/dxclient WORK_DIR=/tmp/dxop
# The DXClient user of the base image (uid 1000); /dxclient is writable by it (DXClient keeps its settings there)
USER 1000:1000
WORKDIR /dxclient
ENTRYPOINT ["/usr/local/bin/manager"]
