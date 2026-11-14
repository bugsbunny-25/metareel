// expo-sharing (share extension) and expo-widgets (widget extension) both
// add a group with an "Info.plist" through the xcode library, which reuses an
// existing file reference with the same path. The second extension's group
// then points at the first one's Info.plist, and Xcode refuses the project
// ("The reference "Info.plist" is a member of more than one group").
//
// Each target reads its plist from INFOPLIST_FILE (its own path), so the
// reference only matters to the project navigator: give every group after the
// first its own file reference.
const { withXcodeProject } = require('expo/config-plugins');

function uuid(project) {
  return project.generateUuid();
}

module.exports = function withUniqueExtensionPlists(config) {
  return withXcodeProject(config, (cfg) => {
    const project = cfg.modResults;
    const objects = project.hash.project.objects;
    const groups = objects.PBXGroup || {};
    const files = objects.PBXFileReference || {};
    const seen = new Set();
    for (const [key, group] of Object.entries(groups)) {
      if (key.endsWith('_comment') || !group || !Array.isArray(group.children)) continue;
      group.children = group.children.map((child) => {
        const ref = files[child.value];
        const isPlist = ref && typeof ref === 'object' && String(ref.path).replace(/"/g, '') === 'Info.plist';
        if (!isPlist) return child;
        if (!seen.has(child.value)) {
          seen.add(child.value);
          return child;
        }
        const id = uuid(project);
        files[id] = { ...ref };
        files[`${id}_comment`] = 'Info.plist';
        seen.add(id);
        return { value: id, comment: 'Info.plist' };
      });
    }
    return cfg;
  });
};
