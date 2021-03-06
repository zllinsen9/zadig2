/*
Copyright 2021 The KodeRover Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhook

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	"go.uber.org/zap"

	commonmodels "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	commonrepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
	gitservice "github.com/koderover/zadig/pkg/microservice/aslan/core/common/service/git"
	"github.com/koderover/zadig/pkg/setting"
	"github.com/koderover/zadig/pkg/tool/codehub"
	e "github.com/koderover/zadig/pkg/tool/errors"
)

func ProcessCodehubHook(payload []byte, req *http.Request, requestID string, log *zap.SugaredLogger) error {
	token := req.Header.Get("X-Codehub-Token")
	secret := gitservice.GetHookSecret()

	if secret != "" && token != secret {
		return errors.New("token is illegal")
	}

	eventType := codehub.HookEventType(req)
	event, err := codehub.ParseHook(eventType, payload)
	if err != nil {
		log.Warnf("unexpected event type: %s", eventType)
		return nil
	}

	forwardedProto := req.Header.Get("X-Forwarded-Proto")
	forwardedHost := req.Header.Get("X-Forwarded-Host")
	baseURI := fmt.Sprintf("%s://%s", forwardedProto, forwardedHost)

	var pushEvent *codehub.PushEvent
	var errorList = &multierror.Error{}
	switch event := event.(type) {
	case *codehub.PushEvent:
		pushEvent = event
		if len(pushEvent.Commits) > 0 {
			webhookUser := &commonmodels.WebHookUser{
				Domain:    req.Header.Get("X-Forwarded-Host"),
				UserName:  pushEvent.Commits[0].Author.Name,
				Email:     pushEvent.Commits[0].Author.Email,
				Source:    setting.SourceFromCodeHub,
				CreatedAt: time.Now().Unix(),
			}
			commonrepo.NewWebHookUserColl().Upsert(webhookUser)
		}

		if err = updateServiceTemplateByCodehubPushEvent(pushEvent, log); err != nil {
			errorList = multierror.Append(errorList, err)
		}
	}

	//???????????????webhook
	if err = TriggerWorkflowByCodehubEvent(event, baseURI, requestID, log); err != nil {
		errorList = multierror.Append(errorList, err)
	}

	return errorList.ErrorOrNil()
}

func updateServiceTemplateByCodehubPushEvent(event *codehub.PushEvent, log *zap.SugaredLogger) error {
	log.Infof("EVENT: CODEHUB WEBHOOK UPDATING SERVICE TEMPLATE")
	serviceTmpls, err := GetCodehubServiceTemplates()
	if err != nil {
		log.Errorf("Failed to get codehub service templates, error: %v", err)
		return err
	}

	errs := &multierror.Error{}
	for _, service := range serviceTmpls {
		srcPath := service.SrcPath
		_, _, _, _, path, _, err := GetOwnerRepoBranchPath(srcPath)
		if err != nil {
			errs = multierror.Append(errs, err)
		}
		// ??????PushEvent???Diffs?????????????????????????????????src_path
		affected := false
		fileNames := []string{}
		for _, commit := range event.Commits {
			fileNames = append(fileNames, commit.Added...)
			fileNames = append(fileNames, commit.Modified...)
			fileNames = append(fileNames, commit.Removed...)
		}

		for _, fileName := range fileNames {
			if strings.Contains(fileName, path) {
				affected = true
				break
			}
		}

		if affected {
			log.Infof("Started to sync service template %s from codehub %s", service.ServiceName, service.SrcPath)
			//TODO: ????????????
			service.CreateBy = "system"
			err := SyncServiceTemplateFromCodehub(service, log)
			if err != nil {
				log.Errorf("SyncServiceTemplateFromCodehub failed, error: %v", err)
				errs = multierror.Append(errs, err)
			}
		} else {
			log.Infof("Service template %s from codehub %s is not affected, no sync", service.ServiceName, service.SrcPath)
		}

	}
	return errs.ErrorOrNil()
}

func GetCodehubServiceTemplates() ([]*commonmodels.Service, error) {
	opt := &commonrepo.ServiceListOption{
		Type:   setting.K8SDeployType,
		Source: setting.SourceFromCodeHub,
	}
	return commonrepo.NewServiceColl().ListMaxRevisions(opt)
}

// SyncServiceTemplateFromCodehub Force to sync Service Template to latest commit and content,
// Notes: if remains the same, quit sync; if updates, revision +1
func SyncServiceTemplateFromCodehub(service *commonmodels.Service, log *zap.SugaredLogger) error {
	// ????????????Source???????????????Source????????????gitlab???????????????
	if service.Source != setting.SourceFromCodeHub {
		return fmt.Errorf("service template is not from codehub")
	}
	// ????????????Commit???SHA
	var before string
	if service.Commit != nil {
		before = service.Commit.SHA
	}
	// Sync?????????Commit???SHA
	var after string
	err := syncCodehubLatestCommit(service)
	if err != nil {
		return err
	}
	after = service.Commit.SHA
	// ????????????????????????Sync??????
	if before == after {
		log.Infof("Before and after SHA: %s remains the same, no need to sync", before)
		// ????????????
		return nil
	}
	// ???Ensure??????????????????source?????????source??? codehub???????????? codehub ?????????service???
	if err := fillServiceTmpl(setting.WebhookTaskCreator, service, log); err != nil {
		log.Errorf("ensureServiceTmpl error: %+v", err)
		return e.ErrValidateTemplate.AddDesc(err.Error())
	}

	log.Infof("End of sync service template %s from codehub path %s", service.ServiceName, service.SrcPath)
	return nil
}
