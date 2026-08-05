# Build stage: compile the Go app. DuckDB requires CGO, so we build with a
# C compiler (the official golang image includes gcc). The duckdb-go binding
# ships a prebuilt static lib, so this is a link, not a DuckDB source compile.
#
# Pin the build stage to the SAME glibc as the runtime (Debian bookworm, glibc
# 2.36). The default golang:1.25 tag now tracks a newer OS (glibc >= 2.38),
# which produces a CGO binary that cannot run on bookworm-slim.
FROM golang:1.25-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /out/kingsmarch-koinery .

# Runtime stage: minimal Debian. The CGO binary needs glibc + libstdc++/libgcc
# (DuckDB), which the slim image doesn't include, so we install them.
FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       libc6 libstdc++6 libgcc-s1 \
       ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/kingsmarch-koinery /kingsmarch-koinery

# Long-running daemon: no ports exposed (outbound only).
ENTRYPOINT ["/kingsmarch-koinery"]
