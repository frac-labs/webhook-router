# webhook-router

Tracked by clawdiovascular#17 (D1).

HTTP receiver → verify HMAC → normalize → fan out to subscribers (Mattermost, Plane, Hermes, Frac).

v0.1.0 is a flag-only scaffold; all POST handlers return 501. Real logic lands in follow-up PRs.

## Build

```
go build ./...
```

## Run

```
./router --listen :8080 --subscribers /etc/webhook-router/subscribers.yaml
```

## Image

`docker.io/ctrahey/webhook-router:<tag>` (DockerHub; not GHCR — see frac-labs-ghcr-to-dockerhub-migration).
