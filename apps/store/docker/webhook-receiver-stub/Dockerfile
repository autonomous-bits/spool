FROM node:lts-alpine
WORKDIR /usr/src/app
COPY server.mjs cert.pem key.pem ./
EXPOSE 4443
CMD ["node", "server.mjs"]
