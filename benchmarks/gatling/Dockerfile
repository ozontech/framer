FROM openjdk:17-oracle
WORKDIR /app
COPY . /app
RUN ./gradlew
ENTRYPOINT ["./gradlew", "gatlingRun-bench.BenchKt"]
