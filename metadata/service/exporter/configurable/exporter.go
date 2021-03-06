/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package configurable

import (
	"context"
	"sync"
)

import (
	"github.com/apache/dubbo-go/common"
	"github.com/apache/dubbo-go/common/constant"
	"github.com/apache/dubbo-go/common/logger"
	"github.com/apache/dubbo-go/config"
	"github.com/apache/dubbo-go/metadata/service"
	"github.com/apache/dubbo-go/metadata/service/exporter"
)

// MetadataServiceExporter is the ConfigurableMetadataServiceExporter which implement MetadataServiceExporter interface
type MetadataServiceExporter struct {
	serviceConfig   *config.ServiceConfig
	lock            sync.RWMutex
	metadataService service.MetadataService
}

// NewMetadataServiceExporter will return a service_exporter.MetadataServiceExporter with the specified  metadata service
func NewMetadataServiceExporter(metadataService service.MetadataService) exporter.MetadataServiceExporter {
	return &MetadataServiceExporter{
		metadataService: metadataService,
	}
}

// Export will export the metadataService
func (exporter *MetadataServiceExporter) Export() error {
	if !exporter.IsExported() {

		serviceConfig := config.NewServiceConfig("MetadataService", context.Background())
		serviceConfig.Protocol = constant.DEFAULT_PROTOCOL
		serviceConfig.Protocols = map[string]*config.ProtocolConfig{
			constant.DEFAULT_PROTOCOL: generateMetadataProtocol(),
		}
		serviceConfig.InterfaceName = constant.METADATA_SERVICE_NAME
		serviceConfig.Group = config.GetApplicationConfig().Name
		serviceConfig.Version = exporter.metadataService.Version()

		var err error
		func() {
			exporter.lock.Lock()
			defer exporter.lock.Unlock()
			exporter.serviceConfig = serviceConfig
			exporter.serviceConfig.Implement(exporter.metadataService)
			err = exporter.serviceConfig.Export()
		}()

		logger.Infof("The MetadataService exports urls : %v ", exporter.serviceConfig.GetExportedUrls())
		return err
	}
	logger.Warnf("The MetadataService has been exported : %v ", exporter.serviceConfig.GetExportedUrls())
	return nil
}

// Unexport will unexport the metadataService
func (exporter *MetadataServiceExporter) Unexport() {
	if exporter.IsExported() {
		exporter.serviceConfig.Unexport()
	}
}

// GetExportedURLs will return the urls that export use.
// Notice！The exported url is not same as url in registry , for example it lack the ip.
func (exporter *MetadataServiceExporter) GetExportedURLs() []*common.URL {
	return exporter.serviceConfig.GetExportedUrls()
}

// isExported will return is metadataServiceExporter exported or not
func (exporter *MetadataServiceExporter) IsExported() bool {
	exporter.lock.RLock()
	defer exporter.lock.RUnlock()
	return exporter.serviceConfig != nil && exporter.serviceConfig.IsExport()
}

// generateMetadataProtocol will return a default ProtocolConfig
func generateMetadataProtocol() *config.ProtocolConfig {
	return &config.ProtocolConfig{
		Name: constant.DEFAULT_PROTOCOL,
		Port: "20000",
	}
}
