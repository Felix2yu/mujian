.PHONY: dev build docker clean

dev-frontend:
	cd frontend && pnpm run dev

dev-backend:
	./dev.sh

build:
	cd frontend && pnpm run build
	cp -r frontend/dist backend/dist
	cd backend && CGO_ENABLED=1 go build -o mujian .

docker:
	docker compose up -d

clean:
	rm -rf frontend/dist backend/dist backend/mujian backend/data
