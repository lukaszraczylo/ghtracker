FROM node:22-alpine AS ui
WORKDIR /app
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
RUN pnpm install --frozen-lockfile
COPY index.html vite.config.ts tsconfig.json ./
COPY scripts ./scripts
COPY src ./src
RUN pnpm build

FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /app/dist ./dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /ghtracker .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /ghtracker /ghtracker
EXPOSE 8080
ENTRYPOINT ["/ghtracker", "-config", "/config/config.yaml"]
