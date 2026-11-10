# BDD

O cenário `order_saga.feature` descreve o fluxo completo entre OS, Billing e Execution, incluindo o caminho de compensação. A execução automatizada contra o ambiente integrado deve subir `docker-compose.phase4.yml` e usar os contratos HTTP/RabbitMQ descritos no README raiz.
