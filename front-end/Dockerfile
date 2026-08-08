FROM node:lts-alpine
WORKDIR /app
COPY package.json .
RUN corepack enable
RUN pnpm install
COPY . .
RUN pnpm run build
CMD ["pnpm", "run", "start"]