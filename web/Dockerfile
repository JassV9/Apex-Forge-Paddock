# Development image: runs the Vite dev server for `make dev`. The read-only demo deployment
# (phase 7) gets its own production image.
FROM node:24-alpine
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
EXPOSE 5173
CMD ["npm", "run", "dev"]
