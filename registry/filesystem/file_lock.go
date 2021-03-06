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

package filesystem

import (
	"github.com/apache/dubbo-go/common/logger"
	perrors "github.com/pkg/errors"
	"io"
	"os"
	"syscall"
)

type Filelock struct {
	LockFile *File
	lock     *os.File
}

// 释放文件锁
func (f *Filelock) Release() {
	if f != nil && f.lock != nil {
		f.lock.Close()
		os.Remove(f.LockFile.Path)
	}
}

// 上锁，配合 defer f.Unlock() 来使用
func (f *Filelock) Lock() (e error) {
	if f == nil {
		return perrors.Errorf("cannot use lock on a nil flock")
	}
	return syscall.Flock(int(f.lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// 解锁
func (f *Filelock) Unlock() {
	if f != nil {
		syscall.Flock(int(f.lock.Fd()), syscall.LOCK_UN)
	}
}

func (f *Filelock)WriteFile(content string){
	//创建或截断打开文件
	file,err := os.Open(f.LockFile.Path)
	if err != nil{
		return
	}

	defer file.Close()

	file.WriteString(content)
}

func (f *Filelock)ReadFile()(string, error){
	//打开文件
	file,err := os.Open(f.LockFile.Path)
	if err != nil{
		return "", err
	}
	defer file.Close()

	var buf []byte = make([]byte ,1024)
	_,err2 := file.Read(buf)
	if err2 != nil && err2 != io.EOF{
		logger.Errorf("read file error : %v", err)
		return "", err2
	}
	return string(buf), nil
}
