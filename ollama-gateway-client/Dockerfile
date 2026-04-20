FROM nginx:alpine

COPY docker-entrypoint-client.sh /docker-entrypoint-client.sh
RUN chmod +x /docker-entrypoint-client.sh

EXPOSE 11434

ENTRYPOINT ["/docker-entrypoint-client.sh"]
