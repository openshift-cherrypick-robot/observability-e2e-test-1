package main_test

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/slack-go/slack"

	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/open-cluster-management/observability-e2e-test/utils"
)

var _ = Describe("Observability:", func() {
	BeforeEach(func() {
		hubClient = utils.NewKubeClient(
			testOptions.HubCluster.MasterURL,
			testOptions.KubeConfig,
			testOptions.HubCluster.KubeContext)

		dynClient = utils.NewKubeClientDynamic(
			testOptions.HubCluster.MasterURL,
			testOptions.KubeConfig,
			testOptions.HubCluster.KubeContext)
	})
	statefulset := [...]string{"alertmanager", "observability-observatorium-thanos-rule"}
	configmap := [...]string{"thanos-ruler-default-rules", "thanos-ruler-custom-rules"}
	secret := "alertmanager-config"

	It("should have the expected statefulsets (alert/g0)", func() {
		By("Checking if STS: Alertmanager and observability-observatorium-thanos-rule exist")
		for _, name := range statefulset {
			sts, err := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).Get(name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(sts.Spec.Template.Spec.Volumes)).Should(BeNumerically(">", 0))

			if sts.GetName() == "alertmanager" {
				By("The statefulset: " + sts.GetName() + " should have the appropriate secret mounted")
				Expect(sts.Spec.Template.Spec.Volumes[0].Secret.SecretName).To(Equal("alertmanager-config"))
			}

			if sts.GetName() == "observability-observatorium-thanos-rule" {
				By("The statefulset: " + sts.GetName() + " should have the appropriate configmap mounted")
				Expect(sts.Spec.Template.Spec.Volumes[0].ConfigMap.Name).To(Equal("thanos-ruler-default-rules"))
			}
		}
	})

	It("should have the expected configmap (alert/g0)", func() {
		By("Checking if CM: thanos-ruler-default-rules is existed")
		cm, err := hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Get(configmap[0], metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(cm.ResourceVersion).ShouldNot(BeEmpty())
		klog.V(3).Infof("Configmap %s does exist", configmap[0])
	})

	It("should not have the CM: thanos-ruler-custom-rules (alert/g0)", func() {
		By("Checking if CM: thanos-ruler-custom-rules not existed")
		_, err := hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Get(configmap[1], metav1.GetOptions{})

		if err == nil {
			err = fmt.Errorf("%s exist within the namespace env", configmap[1])
			Expect(err).NotTo(HaveOccurred())
		}

		Expect(err).To(HaveOccurred())
		klog.V(3).Infof("Configmap %s does not exist", configmap[1])
	})

	It("[P1,Sev1,observability]should have custom alert generated (alert/g0)", func() {
		By("Creating custom alert rules")
		cm := utils.CreateCustomAlertRuleYaml("instance:node_memory_utilisation:ratio * 100 > 0")
		Expect(utils.Apply(testOptions.HubCluster.MasterURL, testOptions.KubeConfig, testOptions.HubCluster.KubeContext, cm)).NotTo(HaveOccurred())

		By("Checking alert generated")
		Eventually(func() error {
			err, _ := utils.ContainManagedClusterMetric(testOptions, `ALERTS{alertname="NodeOutOfMemory"}`, "2m", []string{`"__name__":"ALERTS"`, `"alertname":"NodeOutOfMemory"`})
			return err
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("[P1,Sev1,observability]should have custom alert updated (alert/g0)", func() {
		By("Updating custom alert rules")
		cm := utils.CreateCustomAlertRuleYaml("instance:node_memory_utilisation:ratio * 100 < 0")
		Expect(utils.Apply(testOptions.HubCluster.MasterURL, testOptions.KubeConfig, testOptions.HubCluster.KubeContext, cm)).NotTo(HaveOccurred())

		By("Checking alert generated")
		Eventually(func() error {
			err, _ := utils.ContainManagedClusterMetric(testOptions, `ALERTS{alertname="NodeOutOfMemory"}`, "1m", []string{`"__name__":"ALERTS"`, `"alertname":"NodeOutOfMemory"`})
			return err
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(MatchError("Failed to find metric name from response"))
	})

	It("should have the expected secret (alert/g0)", func() {
		By("Checking if SECRETS: alertmanager-config is existed")
		secret, err := hubClient.CoreV1().Secrets(MCO_NAMESPACE).Get(secret, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(secret.GetName()).To(Equal("alertmanager-config"))
		klog.V(3).Infof("Successfully got secret: %s", secret.GetName())
	})

	It("should modify the SECRET: alertmanager-config (alert/g0)", func() {
		By("Editing the secret, we should be able to add the third partying tools integrations")
		secret := utils.CreateCustomAlertConfigYaml(testOptions.HubCluster.BaseDomain)

		Expect(utils.Apply(testOptions.HubCluster.MasterURL, testOptions.KubeConfig, testOptions.HubCluster.KubeContext, secret)).NotTo(HaveOccurred())
		klog.V(3).Infof("Successfully modified the secret: alertmanager-config")
	})

	It("should verify that the alerts are created (alert/g0)", func() {
		By("Checking that alertmanager and thanos-rule pods are running")
		podList, err := hubClient.CoreV1().Pods(MCO_NAMESPACE).List(metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())

		for _, pod := range podList.Items {
			if strings.Contains(pod.GetName(), "alertmanager") || strings.Contains(pod.GetName(), "thanos-rule") {
				Eventually(func() error {
					p, err := hubClient.CoreV1().Pods(MCO_NAMESPACE).Get(pod.GetName(), metav1.GetOptions{})
					Expect(err).NotTo(HaveOccurred())

					if string(p.Status.Phase) != "Running" {
						klog.V(3).Infof("%s is (%s)", p.GetName(), string(p.Status.Phase))
						return fmt.Errorf("%s is waiting to run", p.GetName())
					}

					Expect(string(p.Status.Phase)).To(Equal("Running"))
					klog.V(3).Infof("%s is (%s)", p.GetName(), string(p.Status.Phase))
					return nil
				}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
			}
		}

		By("Exporting slack bot oauth token, we can view the channel that will hold the alert notifications")
		if os.Getenv("SLACK_BOT_OUATH_TOKEN") != "" && os.Getenv("SLACK_BOT_ID") != "" && os.Getenv("SLACK_CHANNEL_ID") != "" {
			slackAPI := slack.New(os.Getenv("SLACK_BOT_OUATH_TOKEN"))

			bot, err := slackAPI.GetBotInfo(os.Getenv("SLACK_BOT_ID"))
			Expect(err).NotTo(HaveOccurred())
			Expect(bot.Name).Should(Equal("TestingObserv"))
			klog.V(3).Infof("Found slack bot: %s", bot.Name)

			channel, err := slackAPI.GetConversationInfo(os.Getenv("SLACK_CHANNEL_ID"), false)
			Expect(err).NotTo(HaveOccurred())
			Expect(channel.Name).Should(Equal("team-observability-test"))
			klog.V(3).Infof("Found slack channel for testing: %s", channel.Name)

			history, err := slackAPI.GetConversationHistory(&slack.GetConversationHistoryParameters{ChannelID: os.Getenv("SLACK_CHANNEL_ID"), Limit: 3})
			Expect(err).NotTo(HaveOccurred())
			Expect(history.Ok).Should(Equal(true))

			Expect(len(history.Messages)).Should(BeNumerically(">", 0))
			klog.V(3).Infof("Found slack messages")
			for _, msg := range history.Messages {
				klog.Info(msg.Attachments[0].Text)
			}

			timestamp := history.Messages[0].Timestamp

			var (
				retry = 0
				max   = 100
			)

			Eventually(func() error {
				history, err := slackAPI.GetConversationHistory(&slack.GetConversationHistoryParameters{ChannelID: os.Getenv("SLACK_CHANNEL_ID"), Limit: 2})
				Expect(err).NotTo(HaveOccurred())
				Expect(history.Ok).Should(Equal(true))

				klog.V(5).Infof("Latest alert (%s): "+history.Messages[0].Attachments[0].Text, history.Messages[0].Timestamp)

				if retry == max {
					err := fmt.Errorf("Max retry limit has been reached... failing test.")
					klog.V(3).Infof("Max retry limit has been reached... failing test.")
					Expect(err).NotTo(HaveOccurred())
				}

				if timestamp == history.Messages[0].Timestamp || !strings.Contains(history.Messages[0].Attachments[0].Title, "NodeOutOfMemory") {
					klog.V(3).Infof("Waiting for new alert.. Retrying (%d/%d)", retry, max)
					retry += 1
					return fmt.Errorf("No new slack alerts has been created.")
				}

				return nil
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
		} else {
			err := fmt.Errorf(`Error: Missing a required exported variable
				SLACK_BOT_OUATH_TOKEN: %s
				SLACK_BOT_ID: %s
				SLACK_CHANNEL_ID: %s`,
				os.Getenv("SLACK_BOT_OUATH_TOKEN"), os.Getenv("SLACK_BOT_ID"), os.Getenv("SLACK_CHANNEL_ID"),
			)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("should delete the created configmap (alert/g0)", func() {
		err := hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Delete(configmap[1], &metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())

		klog.V(3).Infof("Successfully deleted CM: thanos-ruler-custom-rules")
	})
})
