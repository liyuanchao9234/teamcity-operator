package resource

import (
	"fmt"
	. "git.jetbrains.team/tch/teamcity-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"strings"
)

var _ = Describe("UpdateWithROUtils", func() {
	Context("BuildRoNode", func() {
		It("creates a node with the correct name and resources from main node", func() {
			instance := &TeamCity{
				Spec: TeamCitySpec{
					MainNode: Node{
						Name: "main-node",
						Spec: NodeSpec{
							Requests: v12.ResourceList{
								"cpu":    resource.MustParse("1000m"),
								"memory": resource.MustParse("2Gi"),
							},
						},
					},
				},
			}

			roNode := BuildRoNode(instance, "main-node-update-replica")

			Expect(roNode.Name).To(Equal("main-node-update-replica"))
			Expect(roNode.Spec.Requests["cpu"]).To(Equal(resource.MustParse("1000m")))
			Expect(roNode.Spec.Requests["memory"]).To(Equal(resource.MustParse("2Gi")))
		})

		It("copies serviceName from main node", func() {
			instance := &TeamCity{
				Spec: TeamCitySpec{
					MainNode: Node{
						Name: "main-node",
						Spec: NodeSpec{
							Requests: v12.ResourceList{
								"cpu":    resource.MustParse("1000m"),
								"memory": resource.MustParse("2Gi"),
							},
							ServiceName: "main-headless-svc",
						},
					},
				},
			}

			roNode := BuildRoNode(instance, "main-node-update-replica")
			Expect(roNode.Spec.ServiceName).To(Equal("main-headless-svc"))
		})
	})

	Context("GetROStatefulSetNamespacedName", func() {
		It("returns the correct namespaced name with postfix", func() {
			instance := &TeamCity{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-tc",
					Namespace: "test-namespace",
				},
				Spec: TeamCitySpec{
					MainNode: Node{
						Name: "main-node",
					},
				},
			}

			result := GetROStatefulSetNamespacedName(instance)

			Expect(result.Name).To(Equal("main-node-update-replica"))
			Expect(result.Namespace).To(Equal("test-namespace"))
		})
	})

	Context("BuildROStatefulSet", func() {
		It("creates a StatefulSet with correct name and labels", func() {
			instance := &TeamCity{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-tc",
					Namespace: "test-namespace",
				},
				Spec: TeamCitySpec{
					MainNode: Node{
						Name: "main-node",
						Spec: NodeSpec{
							Requests: v12.ResourceList{
								"cpu":    resource.MustParse("500m"),
								"memory": resource.MustParse("1Gi"),
							},
						},
					},
				},
			}

			roStatefulSet := BuildROStatefulSet(instance)

			Expect(roStatefulSet.Name).To(Equal("main-node-update-replica"))
			Expect(roStatefulSet.Namespace).To(Equal("test-namespace"))
			Expect(roStatefulSet.Labels["teamcity.jetbrains.com/role"]).To(Equal(RoNodeRole))
		})
	})

	Context("ChangesRequireNodeStatefulSetRestart", func() {
		var instance *TeamCity
		var node Node

		BeforeEach(func() {
			instance = &TeamCity{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-tc",
					Namespace: "test-namespace",
				},
				Spec: TeamCitySpec{
					Image: "jetbrains/teamcity-server:latest",
					DataDirVolumeClaim: CustomPersistentVolumeClaim{
						Name: "data-dir",
						VolumeMount: v12.VolumeMount{
							Name:      "data",
							MountPath: "/data/teamcity",
						},
					},
					MainNode: Node{
						Name: "main-node",
						Spec: NodeSpec{
							Requests: v12.ResourceList{
								"cpu":    resource.MustParse("500m"),
								"memory": resource.MustParse("1Gi"),
							},
						},
					},
				},
			}
			node = instance.Spec.MainNode
		})

		It("returns true when image changes", func() {
			existing := &v1.StatefulSet{
				Spec: v1.StatefulSetSpec{
					Template: v12.PodTemplateSpec{
						Spec: v12.PodSpec{
							Containers: []v12.Container{
								{
									Image: "jetbrains/teamcity-server:old",
								},
							},
						},
					},
				},
			}

			result := ChangesRequireNodeStatefulSetRestart(instance, node, existing)
			Expect(result).To(BeTrue())
		})

		It("returns true when resources change", func() {
			existing := &v1.StatefulSet{
				Spec: v1.StatefulSetSpec{
					Template: v12.PodTemplateSpec{
						Spec: v12.PodSpec{
							Containers: []v12.Container{
								{
									Image: "jetbrains/teamcity-server:latest",
									Resources: v12.ResourceRequirements{
										Requests: v12.ResourceList{
											"cpu":    resource.MustParse("200m"),
											"memory": resource.MustParse("512Mi"),
										},
									},
								},
							},
						},
					},
				},
			}

			result := ChangesRequireNodeStatefulSetRestart(instance, node, existing)
			Expect(result).To(BeTrue())
		})
	})

	Context("UpdateROStatefulSet", func() {
		It("sets TEAMCITY_SERVER_OPTS rootURL with main node serviceName", func() {
			scheme := runtime.NewScheme()
			Expect(AddToScheme(scheme)).To(Succeed())

			instance := &TeamCity{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-tc",
					Namespace: "test-namespace",
				},
				Spec: TeamCitySpec{
					Image: "jetbrains/teamcity-server:latest",
					DataDirVolumeClaim: CustomPersistentVolumeClaim{
						Name: "data-dir",
						VolumeMount: v12.VolumeMount{
							Name:      "data",
							MountPath: "/data/teamcity",
						},
					},
					MainNode: Node{
						Name: "main-node",
						Spec: NodeSpec{
							Requests: v12.ResourceList{
								"cpu":    resource.MustParse("500m"),
								"memory": resource.MustParse("1Gi"),
							},
							ServiceName: "main-headless-svc",
						},
					},
				},
			}

			mainStatefulSet := &v1.StatefulSet{
				Spec: v1.StatefulSetSpec{
					Template: v12.PodTemplateSpec{
						Spec: v12.PodSpec{
							Containers: []v12.Container{{Name: "teamcity-server"}},
						},
					},
				},
			}

			roStatefulSet := BuildROStatefulSet(instance)
			Expect(UpdateROStatefulSet(scheme, instance, mainStatefulSet, roStatefulSet)).To(Succeed())

			env := roStatefulSet.Spec.Template.Spec.Containers[0].Env
			var serverOpts string
			for _, e := range env {
				if e.Name == "TEAMCITY_SERVER_OPTS" {
					serverOpts = e.Value
					break
				}
			}

			expected := fmt.Sprintf("-Dteamcity.server.rootURL=http://$(POD_NAME).%s.$(POD_NAMESPACE).svc", instance.Spec.MainNode.Spec.ServiceName)
			Expect(strings.Contains(serverOpts, expected)).To(BeTrue())
		})
	})
})
