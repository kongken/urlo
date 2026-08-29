FROM node:22-alpine AS builder
WORKDIR /app

RUN corepack enable

COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

# Build-time public env (inlined by Vite into the bundle).
# Override with: docker build --build-arg VITE_GOOGLE_CLIENT_ID=...
#
# Vite only reads VITE_* from .env* files, not from process.env, so we
# must materialise the value into .env.production before `pnpm build`.
ARG VITE_GOOGLE_CLIENT_ID=""

COPY . .

RUN echo "[build] VITE_GOOGLE_CLIENT_ID length=${#VITE_GOOGLE_CLIENT_ID}" \
 && if [ -z "$VITE_GOOGLE_CLIENT_ID" ]; then \
      echo "[build] WARNING: VITE_GOOGLE_CLIENT_ID is empty — login will be disabled"; \
    else \
      echo "[build] VITE_GOOGLE_CLIENT_ID prefix=$(printf '%s' "$VITE_GOOGLE_CLIENT_ID" | head -c 12)…"; \
      printf 'VITE_GOOGLE_CLIENT_ID=%s\n' "$VITE_GOOGLE_CLIENT_ID" > .env.production; \
      echo "[build] wrote .env.production ($(wc -c < .env.production) bytes)"; \
    fi

RUN pnpm build

# Verify the value made it into the bundle.
RUN echo "[post-build] grep googleusercontent.com in dist/assets:" \
 && (grep -rl 'googleusercontent\.com' dist/assets || echo "  NONE FOUND")

FROM nginx:alpine AS runtime
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
