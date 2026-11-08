-- Consulta de limite (sem pedido) grava o status pre_aprovado.
ALTER TABLE consultas DROP CONSTRAINT consultas_status_check;
ALTER TABLE consultas ADD CONSTRAINT consultas_status_check
    CHECK (status IN ('aprovado', 'negado', 'contraproposta', 'pre_aprovado', 'documento_invalido', 'em_analise'));
