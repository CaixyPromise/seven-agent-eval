# Actual Linux arm64 install acceptance: no repository is mounted in this image.
FROM node:20.19.5-bookworm-slim@sha256:9e70124bd00f47dd023e349cd587132ae61892acc0e47ed641416c3e18f401c3 AS node
FROM python:3.11-slim-bookworm@sha256:2333bd330d12de02514770b3585cad313644316047cdee24a7acfdece6de6efb
COPY --from=node /usr/local/bin/node /usr/local/bin/node
