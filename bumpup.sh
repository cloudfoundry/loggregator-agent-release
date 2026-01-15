for os in linux windows darwin; do
            if [[ ! -d packages/golang-1.25-${os} ]]; then
              echo "golang-1.25-${os} package directory does not exist, skipping"
            elif [[ ! -f packages/golang-1.25-${os}/spec.lock ]]; then
              echo "golang-1.25-${os} is not vendored yet, vendoring now..."
              bosh vendor-package golang-1.25-${os} ../golang-release

              # Add the golang blob to otel-collector-release with the path expected by vendor-package
              if [[ "$os" == "linux" || "$os" == "windows" ]]; then
                expected_blob_path=$(yq eval '.builds | keys | .[0]' .final_builds/packages/golang-1.25-${os}/index.yml 2>/dev/null || echo "")
                if [[ -n "$expected_blob_path" ]]; then
                  # Dynamically find the latest Go version from available blobs
                  if [[ "$os" == "linux" ]]; then
                    GO_BLOB=$(ls ../golang-release/blobs/go*.linux-amd64.tar.gz | sort -V | tail -n 1 | xargs basename 2>/dev/null)
                    if [[ -n "$GO_BLOB" ]]; then
                      bosh add-blob "../golang-release/blobs/$GO_BLOB" "golang-1.25-linux/$expected_blob_path"
                      echo "Added blob $GO_BLOB for golang-1.25-${os} with path: golang-1.25-${os}/$expected_blob_path"
                    else
                      echo "Warning: No Go linux blob found in golang-release"
                    fi
                  elif [[ "$os" == "windows" ]]; then
                    GO_BLOB=$(ls ../golang-release/blobs/go*.windows-amd64.zip | sort -V | tail -n 1 | xargs basename 2>/dev/null)
                    if [[ -n "$GO_BLOB" ]]; then
                      bosh add-blob "../golang-release/blobs/$GO_BLOB" "golang-1.25-windows/$expected_blob_path"
                      echo "Added blob $GO_BLOB for golang-1.25-${os} with path: golang-1.25-${os}/$expected_blob_path"
                    else
                      echo "Warning: No Go windows blob found in golang-release"
                    fi
                  fi
                else
                  echo "Warning: Could not determine expected blob path for golang-1.25-${os}"
                fi
              fi

              git add .final_builds/packages/golang-1.25-${os}/index.yml
              git add packages/golang-1.25-${os}/spec.lock
            else
              echo "golang-1.25-${os} is already vendored, skipping"
            fi
          done

          # Upload blobs to GCP blobstore
          echo "Uploading blobs to GCP blobstore..."
          bosh upload-blobs
          bosh blobs

          # Stage all changes including modified, deleted, and new files
          echo "Staging all changes..."
          git add -A

          if [ ! -z "$(git status --porcelain)" ]; then
            git commit -m "bump-golang to ${GOLANG_VERSION}"
          else
            echo "No changes to commit."
          fi
