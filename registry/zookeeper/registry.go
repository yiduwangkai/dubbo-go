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

package zookeeper

import (
	"fmt"
	"net/url"
	"sync"
	"time"
)

import (
	"github.com/dubbogo/go-zookeeper/zk"
	perrors "github.com/pkg/errors"
)

import (
	"github.com/apache/dubbo-go/common"
	"github.com/apache/dubbo-go/common/constant"
	"github.com/apache/dubbo-go/common/extension"
	"github.com/apache/dubbo-go/common/logger"
	"github.com/apache/dubbo-go/registry"
	"github.com/apache/dubbo-go/remoting/zookeeper"
)

const (
	// RegistryZkClient zk client name
	RegistryZkClient = "zk registry"
)

func init() {
	extension.SetRegistry("zookeeper", newZkRegistry)
}

/////////////////////////////////////
// zookeeper registry
/////////////////////////////////////

type zkRegistry struct {
	registry.BaseRegistry
	client       *zookeeper.ZookeeperClient
	listenerLock sync.Mutex
	listener     *zookeeper.ZkEventListener
	dataListener *RegistryDataListener
	cltLock      sync.Mutex
	//for provider
	zkPath map[string]int // key = protocol://ip:port/interface
}

func newZkRegistry(url *common.URL) (registry.Registry, error) {
	var (
		err error
		r   *zkRegistry
	)
	r = &zkRegistry{
		zkPath: make(map[string]int),
	}
	r.InitBaseRegistry(url, r)

	err = zookeeper.ValidateZookeeperClient(r, zookeeper.WithZkName(RegistryZkClient))
	if err != nil {
		return nil, err
	}

	go zookeeper.HandleClientRestart(r)

	r.listener = zookeeper.NewZkEventListener(r.client)

	r.dataListener = NewRegistryDataListener()

	return r, nil
}

// Options ...
type Options struct {
	client *zookeeper.ZookeeperClient
}

// Option ...
type Option func(*Options)

func newMockZkRegistry(url *common.URL, opts ...zookeeper.Option) (*zk.TestCluster, *zkRegistry, error) {
	var (
		err error
		r   *zkRegistry
		c   *zk.TestCluster
		//event <-chan zk.Event
	)

	r = &zkRegistry{
		zkPath: make(map[string]int),
	}
	r.InitBaseRegistry(url, r)
	c, r.client, _, err = zookeeper.NewMockZookeeperClient("test", 15*time.Second, opts...)
	if err != nil {
		return nil, nil, err
	}
	r.WaitGroup().Add(1) //zk client start successful, then wg +1
	go zookeeper.HandleClientRestart(r)
	r.InitListeners()
	return c, r, nil
}

func (r *zkRegistry) InitListeners() {
	r.listener = zookeeper.NewZkEventListener(r.client)
	newDataListener := NewRegistryDataListener()
	// should recover if dataListener isn't nil before
	if r.dataListener != nil {
		// close all old listener
		oldDataListener := r.dataListener
		oldDataListener.mutex.Lock()
		defer oldDataListener.mutex.Unlock()
		recoverd := r.dataListener.subscribed
		if recoverd != nil && len(recoverd) > 0 {
			// recover all subscribed url
			for conf, oldListener := range recoverd {
				if regConfigListener, ok := oldListener.(*RegistryConfigurationListener); ok {
					regConfigListener.Close()
				}
				newDataListener.SubscribeURL(conf, NewRegistryConfigurationListener(r.client, r))
				go r.listener.ListenServiceEvent(conf, fmt.Sprintf("/dubbo/%s/"+constant.DEFAULT_CATEGORY, url.QueryEscape(conf.Service())), newDataListener)

			}
		}
	}
	r.dataListener = newDataListener
}

func (r *zkRegistry) CreatePath(path string) error {
	return r.ZkClient().Create(path)
}

func (r *zkRegistry) DoRegister(root string, node string) error {
	return r.registerTempZookeeperNode(root, node)
}

func (r *zkRegistry) DoSubscribe(conf *common.URL) (registry.Listener, error) {
	return r.getListener(conf)
}

func (r *zkRegistry) CloseAndNilClient() {
	r.client.Close()
	r.client = nil
}

func (r *zkRegistry) ZkClient() *zookeeper.ZookeeperClient {
	return r.client
}

func (r *zkRegistry) SetZkClient(client *zookeeper.ZookeeperClient) {
	r.client = client
}

func (r *zkRegistry) ZkClientLock() *sync.Mutex {
	return &r.cltLock
}

func (r *zkRegistry) CloseListener() {
	if r.dataListener != nil {
		r.dataListener.Close()
	}
}

func (r *zkRegistry) registerTempZookeeperNode(root string, node string) error {
	var (
		err    error
		zkPath string
	)

	r.cltLock.Lock()
	defer r.cltLock.Unlock()
	err = r.client.Create(root)
	if err != nil {
		logger.Errorf("zk.Create(root{%s}) = err{%v}", root, perrors.WithStack(err))
		return perrors.WithStack(err)
	}

	// try to register the node
	zkPath, err = r.client.RegisterTemp(root, node)
	if err != nil {
		logger.Errorf("Register temp node(root{%s}, node{%s}) = error{%v}", root, node, perrors.WithStack(err))
		if perrors.Cause(err) == zk.ErrNodeExists {
			// should delete the old node
			logger.Info("Register temp node failed, try to delete the old and recreate  (root{%s}, node{%s}) , ignore!", root, node)
			if err = r.client.Delete(zkPath); err == nil {
				_, err = r.client.RegisterTemp(root, node)
			}
			if err != nil {
				logger.Errorf("Recreate the temp node failed, (root{%s}, node{%s}) = error{%v}", root, node, perrors.WithStack(err))
			}
		}
		return perrors.WithMessagef(err, "RegisterTempNode(root{%s}, node{%s})", root, node)
	}
	logger.Debugf("Create a zookeeper node:%s", zkPath)

	return nil
}

func (r *zkRegistry) getListener(conf *common.URL) (*RegistryConfigurationListener, error) {

	var zkListener *RegistryConfigurationListener
	dataListener := r.dataListener
	dataListener.mutex.Lock()
	defer dataListener.mutex.Unlock()
	if r.dataListener.subscribed[conf] != nil {

		zkListener, _ := r.dataListener.subscribed[conf].(*RegistryConfigurationListener)
		if zkListener != nil {
			r.listenerLock.Lock()
			defer r.listenerLock.Unlock()
			if zkListener.isClosed {
				return nil, perrors.New("configListener already been closed")
			} else {
				return zkListener, nil
			}
		}
	}

	zkListener = NewRegistryConfigurationListener(r.client, r)
	if r.listener == nil {
		r.cltLock.Lock()
		client := r.client
		r.cltLock.Unlock()
		if client == nil {
			return nil, perrors.New("zk connection broken")
		}

		// new client & listener
		listener := zookeeper.NewZkEventListener(r.client)

		r.listenerLock.Lock()
		r.listener = listener
		r.listenerLock.Unlock()
	}

	//Interested register to dataconfig.
	r.dataListener.SubscribeURL(conf, zkListener)

	go r.listener.ListenServiceEvent(conf, fmt.Sprintf("/dubbo/%s/"+constant.DEFAULT_CATEGORY, url.QueryEscape(conf.Service())), r.dataListener)

	return zkListener, nil
}
