docker-compose -p deploy build
docker-compose -p deploy up -d --no-recreate blockchain-service1
docker-compose -p deploy up -d --no-recreate blockchain-service2
docker-compose -p deploy up -d --no-recreate blockchain-service3
docker-compose -p deploy up -d --no-recreate blockchain-service4
docker-compose -p deploy up -d --no-recreate blockchain-service5

docker rmi $(docker images | grep "^<none>" | awk '{print $3}') &> /dev/null
docker volume rm $(docker volume ls -qf dangling=true) &> /dev/null
