FROM node:22-alpine AS builder
WORKDIR /app

RUN corepack enable

COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

# Build-time public env (inlined by Vite into the bundle).
# Override with: docker build --build-arg VITE_GOOGLE_CLIENT_ID=...
ARG VITE_GOOGLE_CLIENT_ID=""
ENV VITE_GOOGLE_CLIENT_ID=$VITE_GOOGLE_CLIENT_ID

# Debug: confirm the build arg arrived (length only, never echo the value).
RUN echo "[build] VITE_GOOGLE_CLIENT_ID length=${#VITE_GOOGLE_CLIENT_ID}" \
 && if [ -z "$VITE_GOOGLE_CLIENT_ID" ]; then \
      echo "[build] WARNING: VITE_GOOGLE_CLIENT_ID is empty — login will be disabled"; \
    else \
      echo "[build] VITE_GOOGLE_CLIENT_ID prefix=$(printf '%s' "$VITE_GOOGLE_CLIENT_ID" | head -c 12)…"; \
    fi

COPY . .
RUN pnpm build

# Debug: verify the value made it into the bundle. Prints whether the
# 'apps.googleusercontent.com' suffix is present and the count of matches.
RUN echo "[post-build] occurrences of 'googleusercontent.com' in dist:" \
 && grep -roc 'googleusercontent\.com' dist/assets || true

FROM nginx:alpine AS runtime
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
