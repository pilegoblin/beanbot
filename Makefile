build:
	go build -o bin/beanbot.exe cmd/bot/*

run: clean build
	./bin/beanbot.exe

clean:
	rm -f bin/beanbot.exe
	rm -f bin/beanbot

deploy:
	fly deploy --ha=false