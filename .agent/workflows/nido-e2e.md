---
description: Automated Matrix V2 E2E Testing using Nido VMs
---

Questo workflow automatizza l'esecuzione dei test E2E di Matrix in un ambiente controllato e pulito.

### Prerequisiti
- Nido installato e configurato sull'host.
- Immagine `ubuntu:22.04` scaricata (`nido image pull ubuntu:22.04`).

### Procedura di Automazione

1. **Rilevamento Ambiente**
   Eseguire lo script di bootstrap per garantire l'esistenza della VM di test.
   ```bash
   ./scripts/nido_bootstrap.sh
   ```

2. **Compilazione Matrix**
   Costruire il binario aggiornato dall'host.
   ```bash
   cd matrix-v2 && go build -o matrix ./cmd/matrix/*.go
   ```

3. **Iniezione Test e Binari**
   Sincronizzare il codice di test e il binario nella VM.
   ```bash
   nido ssh matrix-e2e-validator "mkdir -p /tmp/matrix-tests"
   # Nota: Nido non ha un comando 'cp' nativo, usiamo cat/redirect via SSH per file piccoli o scp se abilitato
   cat matrix-v2/matrix | nido ssh matrix-e2e-validator "cat > /tmp/matrix-tests/matrix && chmod +x /tmp/matrix-tests/matrix"
   ```

4. **Esecuzione Test**
   Lanciare la suite di test all'interno della VM.
   ```bash
   nido ssh matrix-e2e-validator "/tmp/matrix-tests/matrix apm list"
   ```

5. **Cleanup (Opzionale)**
   Per mantenere determinismo, distruggere la VM dopo test critici.
   ```bash
   nido stop matrix-e2e-validator
   ```
