.PHONY: dev build docker clean

dev-frontend:
	cd frontend && npm run dev

dev-backend:
	cd backend && go run .

build:
	cd frontend && npm run build
	cp -r frontend/dist backend/dist
	cd backend && CGO_ENABLED=0 go build -o mujian .

docker:
	docker compose build
	docker compose up -d

clean:
	rm -rf frontend/dist backend/dist backend/mujian backend/data
