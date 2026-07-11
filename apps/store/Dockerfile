# syntax=docker/dockerfile:1.4
FROM node:lts-alpine
ENV NODE_ENV=production
WORKDIR /usr/src/app
COPY ["package.json", "package-lock.json*", "npm-shrinkwrap.json*", "./"]
RUN npm install --production --silent && mv node_modules ../
COPY . .
# The webhook-receiver-stub's self-signed cert lives outside this build context (tools/docker/,
# per the repo-root "tools/" convention); compose.yaml supplies it via an additional build
# context named "certs" so NODE_EXTRA_CA_CERTS can trust it during the G14 Docker e2e exercise.
COPY --from=certs cert.pem ./docker/webhook-receiver-stub/cert.pem
EXPOSE 3000
# Pre-create the artifact blob directory before chown so a Docker-named-volume mounted here
# (compose.yaml's spoolstore-artifact-data, Meridian IDEA-85) inherits `node` ownership instead of
# defaulting to root.
RUN mkdir -p /usr/src/app/artifact-data && chown -R node /usr/src/app
USER node
CMD ["npm", "start"]
