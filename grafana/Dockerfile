FROM grafana/grafana:11.6.1

ENV GF_PATHS_PLUGINS=/home/grafana/plugins
RUN mkdir -p $GF_PATHS_PLUGINS && \
    grafana cli --pluginsDir $GF_PATHS_PLUGINS plugins install yesoreyeram-infinity-datasource
