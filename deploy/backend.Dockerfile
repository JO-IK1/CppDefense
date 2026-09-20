FROM golang:1.27-bookworm AS build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cppdefense ./cmd/cppdefense

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/cppdefense /usr/local/bin/cppdefense
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/cppdefense"]
CMD ["api"]
