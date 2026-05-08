#!/usr/bin/env node

/**
 * Utility script to concatenate all non-test Go source files in the project
 * into a single context file for LLM usage.
 */

const fs = require('fs');
const path = require('path');

// Folders we should skip scanning
const EXCLUDED_DIRS = new Set(['.git', '.github', 'node_modules', 'vendor', 'bin']);
const OUTPUT_FILE = path.join(__dirname, '..', 'project_context.txt');
const ROOT_DIR = path.join(__dirname, '..');

function getTargetFiles(dir, fileList = []) {
  const files = fs.readdirSync(dir);

  for (const file of files) {
    const fullPath = path.join(dir, file);
    const stat = fs.statSync(fullPath);

    if (stat.isDirectory()) {
      if (!EXCLUDED_DIRS.has(file)) {
        getTargetFiles(fullPath, fileList);
      }
    } else if (stat.isFile()) {
      const isGoSource = file.endsWith('.go') && !file.endsWith('_test.go');
      const isShSource = file.endsWith('.sh');
      const isDockerfile = file === 'Dockerfile';

      if (isGoSource || isShSource || isDockerfile) {
        fileList.push(fullPath);
      }
    }
  }

  return fileList;
}

function getLanguageId(filePath) {
  if (filePath.endsWith('.go')) return 'go';
  if (filePath.endsWith('.sh')) return 'bash';
  if (path.basename(filePath) === 'Dockerfile') return 'dockerfile';
  return 'text';
}

function buildContext() {
  console.log(`Scanning for Go sources, shell scripts, and Dockerfiles in: ${ROOT_DIR}`);
  const targetFiles = getTargetFiles(ROOT_DIR);
  console.log(`Found ${targetFiles.length} matched context files (excluding tests/md).`);

  let contextContent = '';

  for (const filePath of targetFiles) {
    const relativePath = path.relative(ROOT_DIR, filePath).replace(/\\/g, '/');
    const content = fs.readFileSync(filePath, 'utf8');
    const lang = getLanguageId(filePath);

    contextContent += `### File: ${relativePath}\n`;
    contextContent += `\`\`\`${lang}\n`;
    contextContent += content;
    if (!content.endsWith('\n')) {
      contextContent += '\n';
    }
    contextContent += `\`\`\`\n\n`;
  }

  fs.writeFileSync(OUTPUT_FILE, contextContent, 'utf8');
  console.log(`Successfully generated context file: ${OUTPUT_FILE}`);
}

buildContext();
