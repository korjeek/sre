INSERT INTO services (name, owner, description) VALUES
    ('billing-api', 'payments-team', 'API списаний и счетов'),
    ('auth-gateway', 'platform-team', 'Шлюз аутентификации');

INSERT INTO incidents (service_id, title, description, severity, status) VALUES
    (1, 'Рост 5xx на /charge', 'После деплоя доля 500 выросла до 8%', 'high', 'investigating'),
    (2, 'Таймауты JWT', 'p99 latency > 2s на выдаче токенов', 'medium', 'open');
