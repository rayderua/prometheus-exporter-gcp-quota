## Google Cloud Platform Multi Project Quota Exporter
main idea borrowed from https://github.com/mintel/gcp-quota-exporter

## Usage
Set up a service account in the project you wish to monitor. The account should be given the following permissions:
compute.projects.get
compute.regions.list

## Building and running the exporter
### Create yaml config for exporter like this:
```yaml
---
- project: "google-project"         # Google project name
  regions: []                       # Regions for scrape (scrape all reginos if empty)
  credentials: "credentials.json"   # Service account credentials file path
```

### Build and run locally
```sh
git clone https://github.com/rayderua/prometheus-exporter-gcp-quota.git
cd proemtheus-exporter-gcp-quota
go build -o prometheus-exporter-gcp-quota .
./prometheus-exporter-gcp-quota -config prometheus-exporter-gcp-quota.yaml
```

### Docker build
```shell
docker build -f docker/Dockerfile --tag prometheus-exporter-gcp-quota:latest .
```