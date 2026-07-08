# Sova testsuite

Two suites, run separately.

## `lang/` — shared-side / both-backend

The main language test suite. Every test runs against both the Go
and JS backends. Green here means the language, the standard library
subset, and cross-side wiring all behave identically.

```
cd testsuite/lang
sova test --side both
```

Categories: basics, closures, collections, control, generics,
multipkg, oop, stdlib (path only), types.

## `lang-backend/` — backend-only

Tests for standard-library packages that are backend-only by design
(`std/fs`, and eventually `std/os`, `std/exec`, etc.). Run against
the Go backend only. Frontend-side compilation would emit calls to
extern functions that don't have a `frontend:` mapping — currently
that surfaces as a runtime `ReferenceError` rather than a compile
error, so we run these separately until Sova's frontend-side use of
`on backend` packages becomes a compile-time diagnostic.

```
cd testsuite/lang-backend
sova test --side go
```

Categories: stdlib (`fs`, `s3`).

### Live-service tests

The `stdlib/s3.test.sova` tests need a live S3-compatible endpoint.
They early-return when `SOVA_TEST_S3_ENDPOINT` is empty, so the
suite still runs green without one. To exercise them against a
local MinIO:

```
docker run -d --name minio-sova-test -p 19000:9000 \
    -e MINIO_ROOT_USER=test -e MINIO_ROOT_PASSWORD=testtest \
    quay.io/minio/minio:latest server /data

SOVA_TEST_S3_ENDPOINT=http://localhost:19000 \
SOVA_TEST_S3_KEY=test \
SOVA_TEST_S3_SECRET=testtest \
sova test --side go
```

## Known issues

See [`lang/KNOWN_ISSUES.md`](lang/KNOWN_ISSUES.md) for compiler
issues surfaced by the test suites during their development.
