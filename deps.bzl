load("@bazel_gazelle//:deps.bzl", "go_repository")

def rjsone_dependencies():
    _maybe(
        go_repository,
        name = "com_github_taskcluster_json_e",
        commit = "e7c5057d292797e63f41e90ae469a1007e1f69bc",
        importpath = "github.com/taskcluster/json-e",
    )

    _maybe(
        go_repository,
        name = "com_github_wryun_yaml_1",
        commit = "e5213689ab3ec721209263e51f9edf8615d93085",
        importpath = "github.com/wryun/yaml-1",
    )

    _maybe(
        go_repository,
        name = "in_gopkg_yaml_v2",
        commit = "5420a8b6744d3b0345ab293f6fcba19c978f1183",
        importpath = "gopkg.in/yaml.v2",
    )

def _maybe(repo_rule, name, **kwargs):
    if name not in native.existing_rules():
        repo_rule(name = name, **kwargs)
