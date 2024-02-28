build:
	go build -o bin/beanbot.exe cmd/bot/*

deploy:
	fly deploy --ha=false