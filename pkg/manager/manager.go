// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"errors"
	"net/http"
	"time"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidcontroller "github.com/gardener/etcd-druid/internal/controller"
	druidwebhook "github.com/gardener/etcd-druid/internal/webhook"
	druidwebhookutils "github.com/gardener/etcd-druid/internal/webhook/utils"

	"golang.org/x/exp/slog"
	ctrl "sigs.k8s.io/controller-runtime"
)

// AddToManager adds all etcd-druid controllers, webhooks, and health endpoints to an existing controller manager.
func AddToManager(mgr ctrl.Manager, config *druidconfigv1alpha1.OperatorConfiguration) error {
	slog.Info("registering controllers and webhooks with manager")

	// Register controllers
	if err := druidcontroller.Register(mgr, config.Controllers); err != nil {
		return err
	}

	// Register webhooks
	if err := druidwebhook.Register(mgr, config.Webhooks); err != nil {
		return err
	}

	// Register health and ready endpoints
	if err := registerHealthAndReadyEndpoints(mgr, config); err != nil {
		return err
	}

	return nil
}

func registerHealthAndReadyEndpoints(mgr ctrl.Manager, config *druidconfigv1alpha1.OperatorConfiguration) error {
	slog.Info("Registering ping health check endpoint")
	// Add a health check which always returns true when it is checked
	if err := mgr.AddHealthzCheck("ping", func(_ *http.Request) error { return nil }); err != nil {
		return err
	}

	// Add a readiness check which will pass only when all informers have synced.
	// Typically one would call `HasSync` but that is not exposed out of controller-runtime `cache.Informers`. Instead,
	// give it a context with a very short timeout so that it causes the call to ` cache.WaitForCacheSync` to get executed once.
	// We do not wish to wait longer as the readiness checks should be fast. Once all the cache informers have synced then the
	// readiness check would succeed.
	if err := mgr.AddReadyzCheck("informer-sync", func(_ *http.Request) error {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()
		if !mgr.GetCache().WaitForCacheSync(ctx) {
			return errors.New("informers not synced yet")
		}
		return nil
	}); err != nil {
		return err
	}

	// Add a readiness check for the webhook server
	if druidwebhookutils.AtLeaseOneEnabled(config.Webhooks) {
		slog.Info("Registering webhook-server readiness check endpoint")
		if err := mgr.AddReadyzCheck("webhook-server", mgr.GetWebhookServer().StartedChecker()); err != nil {
			return err
		}
	}
	return nil
}
