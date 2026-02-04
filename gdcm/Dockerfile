# GDCM Container for DICOM Processing with JPEG 2000 Lossless
FROM ubuntu:22.04

# Avoid interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=UTC

# Update system and install dependencies
RUN apt-get update && apt-get install -y \
    # Build essentials
    build-essential \
    cmake \
    swig \
    git \
    wget \
    curl \
    unzip \
    # GDCM dependencies
    libvtk9-dev \
    libvtk9.1 \
    vtk9 \
    # JPEG 2000 support (OpenJPEG)
    libopenjp2-7-dev \
    libopenjp2-tools \
    # Additional image libraries
    libjpeg-dev \
    libpng-dev \
    libtiff-dev \
    # Python for GDCM Python bindings
    python3 \
    python3-dev \
    python3-pip \
    # Network tools
    netcat-openbsd \
    # Cleanup
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /opt

# Download and build GDCM with JPEG 2000 support
RUN git clone https://github.com/malaterre/GDCM.git gdcm-source \
    && cd gdcm-source \
    && git checkout v3.0.24

# Create build directory and configure GDCM
WORKDIR /opt/gdcm-build
RUN cmake /opt/gdcm-source \
    -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_INSTALL_PREFIX=/usr/local \
    -DGDCM_BUILD_SHARED_LIBS=ON \
    -DGDCM_BUILD_APPLICATIONS=OFF \
    -DGDCM_BUILD_EXAMPLES=OFF \
    -DGDCM_USE_VTK=OFF \
    -DGDCM_USE_SYSTEM_OPENJPEG=ON \
    -DGDCM_USE_SYSTEM_LJPEG=OFF \
    -DGDCM_USE_JPEGLS=ON \
    -DGDCM_USE_JPEG2K=ON \
    -DGDCM_USE_OPENJPEG_V2=ON \
    -DGDCM_WRAP_PYTHON=ON \
    -DPython3_EXECUTABLE=/usr/bin/python3

# Build and install GDCM
RUN make -j$(nproc) && make install

# Update library path
RUN echo '/usr/local/lib' > /etc/ld.so.conf.d/gdcm.conf && ldconfig

# Create application directory
WORKDIR /app

# Copy GDCM service files
COPY . .

# Normalize line endings for entrypoint (avoid CRLF issues)
RUN sed -i 's/\r$//' /app/entrypoint.sh

# Install Python dependencies
RUN pip3 install --no-cache-dir \
    flask \
    gunicorn \
    requests \
    python-gdcm \
    pydicom \
    pillow \
    numpy \
    psycopg2-binary

# Create directories for DICOM processing
RUN mkdir -p /app/input /app/output /app/temp /app/logs

# Set proper permissions
RUN chmod +x /app/entrypoint.sh

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
    CMD curl -f http://localhost:3000/health || exit 1

# Expose port for GDCM service
EXPOSE 3000

# Use entrypoint script
ENTRYPOINT ["/app/entrypoint.sh"]
