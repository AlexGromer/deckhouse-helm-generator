FROM registry.example.com/base/jre-alpine:17

COPY target/orders*.jar /app.jar

ENTRYPOINT ["java", "-jar", "/app.jar"]
