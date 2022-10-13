FROM node:latest

# Create app directory
WORKDIR /usr/src/app

# Install app dependencies
COPY . .
RUN npm install

ENTRYPOINT npx hardhat compile
