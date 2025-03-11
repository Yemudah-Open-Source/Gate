# Use the official Golang image as the builder
FROM golang:1.23

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .


# Install air for live reloading
RUN go install github.com/air-verse/air@latest

# Initialize and tidy dependencies
RUN go mod tidy

#RUN CGO_ENABLED=0 GOOS=linux go build -o /gate-service

EXPOSE 6748


CMD ["air"]
