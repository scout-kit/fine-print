FROM golang:1.24-bookworm

RUN apt-get update && apt-get install -y \
    cups cups-client \
    && rm -rf /var/lib/apt/lists/*

RUN go install github.com/air-verse/air@latest

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

EXPOSE 80 5173

CMD ["air"]
