package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

var (
	cfgErrCount        int
	cfgErrDesc         = prometheus.NewDesc("gcp_quota_config_err", "Number errors in exporter config", nil, nil)
	limitDesc          = prometheus.NewDesc("gcp_quota_limit", "quota limits for GCP components", []string{"project", "region", "metric"}, nil)
	usageDesc          = prometheus.NewDesc("gcp_quota_usage", "quota usage for GCP components", []string{"project", "region", "metric"}, nil)
	projectQuotaUpDesc = prometheus.NewDesc("gcp_quota_project_up", "Was the last scrape of the Google Project API successful.", []string{"project"}, nil)
	regionsQuotaUpDesc = prometheus.NewDesc("gcp_quota_regions_up", "Was the last scrape of the Google Regions API successful.", []string{"project", "region"}, nil)
)

func getEnv(key string, defaultVal string) string {
	if envVal, ok := os.LookupEnv(key); ok {
		return envVal
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if envVal, ok := os.LookupEnv(key); ok {
		envBool, err := strconv.ParseBool(envVal)
		if err == nil {
			return envBool
		}
	}
	return defaultVal
}

func getEnvInt64(key string, defaultVal int64) int64 {
	if envVal, ok := os.LookupEnv(key); ok {
		envInt64, err := strconv.ParseInt(envVal, 10, 64)
		if err == nil {
			return envInt64
		}
	}
	return defaultVal
}

type gcpQuota struct {
	Project     string   `json:"Project"`
	Regions     []string `json:"Regions"`
	Credentials string   `json:"Credentials"`
}

type Exporter struct {
	service *compute.Service
	project string
	regions []string
	mutex   sync.RWMutex
}

type configExporter struct {
	service *compute.Service
	mutex   sync.RWMutex
}

func inArray(val interface{}, array interface{}) (result bool) {
	values := reflect.ValueOf(array)
	if reflect.TypeOf(array).Kind() == reflect.Slice || values.Len() > 0 {
		for i := 0; i < values.Len(); i++ {
			if reflect.DeepEqual(val, values.Index(i).Interface()) {
				return true
			}
		}
	}
	return false
}

func (e *configExporter) Describe(ch chan<- *prometheus.Desc) {}

func (e *configExporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()
	ch <- prometheus.MustNewConstMetric(cfgErrDesc, prometheus.GaugeValue, float64(cfgErrCount))
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()

	project, regionList := e.scrape()
	if project != nil {
		for _, quota := range project.Quotas {
			ch <- prometheus.MustNewConstMetric(limitDesc, prometheus.GaugeValue, quota.Limit, e.project, "", quota.Metric)
			ch <- prometheus.MustNewConstMetric(usageDesc, prometheus.GaugeValue, quota.Usage, e.project, "", quota.Metric)
		}
		ch <- prometheus.MustNewConstMetric(projectQuotaUpDesc, prometheus.GaugeValue, 1, e.project)
	} else {
		ch <- prometheus.MustNewConstMetric(projectQuotaUpDesc, prometheus.GaugeValue, 0, e.project)
	}

	var scrapedRegions []string
	if regionList != nil {
		for _, region := range regionList {
			regionName := region.Name
			for _, quota := range region.Quotas {
				ch <- prometheus.MustNewConstMetric(limitDesc, prometheus.GaugeValue, quota.Limit, e.project, regionName, quota.Metric)
				ch <- prometheus.MustNewConstMetric(usageDesc, prometheus.GaugeValue, quota.Usage, e.project, regionName, quota.Metric)
			}
			scrapedRegions = append(scrapedRegions, regionName)
		}
	}

	for _, region := range e.regions {
		if inArray(region, scrapedRegions) {
			ch <- prometheus.MustNewConstMetric(regionsQuotaUpDesc, prometheus.GaugeValue, 1, e.project, region)
		} else {
			ch <- prometheus.MustNewConstMetric(regionsQuotaUpDesc, prometheus.GaugeValue, 0, e.project, region)
		}
	}
}

// scrape connects to the Google API to fetch quota statistics and record them as metrics.
func (e *Exporter) scrape() (prj *compute.Project, rgl []*compute.Region) {

	project, err := e.service.Projects.Get(e.project).Do()
	if err != nil {
		log.Errorf("Failure when querying project quotas: \n%v", err)
		project = nil
	}

	var regionList []*compute.Region

	if len(e.regions) != 0 {
		for _, r := range e.regions {
			region, err := e.service.Regions.Get(e.project, r).Do()
			if err != nil {
				log.Errorf("Failure when querying region quotas: %v", err)
			} else {
				regionList = append(regionList, region)
			}
		}
	} else {
		projectRegions, err := e.service.Regions.List(e.project).Do()
		if err != nil {
			log.Errorf("Failure when querying region quotas: %v", err)
			regionList = nil
		} else {
			for _, r := range projectRegions.Items {
				regionList = append(regionList, r)
			}
		}
	}
	return project, regionList
}

// NewExporter returns an initialised Exporter.
func NewExporter(gcpQuota gcpQuota) (*Exporter, error) {

	ctx := context.Background()

	computeService, err := compute.NewService(ctx, option.WithCredentialsFile(gcpQuota.Credentials))
	if err != nil {
		fmt.Printf("Failure when querying project quotas: %v", err)
	}

	return &Exporter{
		service: computeService,
		project: gcpQuota.Project,
		regions: gcpQuota.Regions,
	}, nil
}

func main() {
	var (
		configPath    = flag.String("config", getEnv("GCP_QUOTA_EXPORTER_CONFIG_", "/etc/prometheus-exporter-gcp-quota.yaml"), "Listen address.")
		listenAddress = flag.String("web.listen-address", getEnv("GCP_QUOTA_EXPORTER_WEB_LISTEN_ADDRESS", "0.0.0.0:9593"), "Address to listen on for web interface and telemetry.")
		metricPath    = flag.String("web.telemetry-path", getEnv("GCP_QUOTA_EXPORTER_WEB_TELEMETRY_PATH", "/metrics"), "Path under which to expose metrics.")
		logFormat     = flag.String("log-format", getEnv("GCP_QUOTA_EXPORTER_LOG_FORMAT", "txt"), "Log format, valid options are txt and json.")
		projectList   = make([]gcpQuota, 256)
	)
	flag.Parse()
	cfgErrCount = 1

	switch *logFormat {
	case "json":
		log.SetFormatter(&log.JSONFormatter{})
	default:
		log.SetFormatter(&log.TextFormatter{})
	}

	config, err := ioutil.ReadFile(*configPath)
	if err != nil {
		log.Fatal("Couldn't read config: ", err)
	}

	err = yaml.Unmarshal(config, &projectList)
	if err != nil {
		log.Fatal("Couldn't parse config: ", err)
	}

	var projectConfigList []string
	for _, project := range projectList {
		if project.Project == "" {
			cfgErrCount++
			continue
		}
		if project.Credentials == "" {
			log.Errorf("Credential not specified for %s", project.Project)
			cfgErrCount++
			continue
		}

		if _, err := os.Stat(project.Credentials); err != nil {
			log.Errorf("Credential file [%s] not found fo %s", project.Credentials, project.Project)
			continue
		}

		if !inArray(project.Project, projectConfigList) {
			exporter, err := NewExporter(project)
			if err != nil {
				log.Fatal(err)
			}
			prometheus.MustRegister(exporter)
			projectConfigList = append(projectConfigList, project.Project)
		} else {
			log.Errorf("Duplicate project [%v] inc %v.", project.Project, configPath)
			cfgErrCount++
		}
	}

	prometheus.MustRegister(&configExporter{})

	log.Infof("Starting gcp quota exporter on %s", *listenAddress)
	log.Infof("Provide metrics on on %s", *metricPath)

	http.Handle(*metricPath, promhttp.Handler())
	err = http.ListenAndServe(*listenAddress, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
