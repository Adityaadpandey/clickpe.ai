'use client'

import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Progress } from '@/components/ui/progress';
import { AlertCircle, CheckCircle2, FileText, Loader2, Upload } from 'lucide-react';
import React, { useCallback, useState } from 'react';

const API_ENDPOINT = 'https://847844lx53.execute-api.us-east-1.amazonaws.com/upload-url';

type StatusType = 'success' | 'error' | 'loading';

interface Status {
  type: StatusType;
  message: string;
}

interface UploadResponse {
  uploadUrl: string;
  fileName: string;
}

export default function LoanUpload() {
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [isDragging, setIsDragging] = useState<boolean>(false);
  const [uploading, setUploading] = useState<boolean>(false);
  const [progress, setProgress] = useState<number>(0);
  const [status, setStatus] = useState<Status | null>(null);

  const formatFileSize = (bytes: number): string => {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(2) + ' KB';
    return (bytes / (1024 * 1024)).toFixed(2) + ' MB';
  };

  const handleFile = useCallback((file: File | undefined) => {
    if (!file) return;

    if (!file.name.endsWith('.csv')) {
      setStatus({ type: 'error', message: 'Please select a CSV file' });
      return;
    }

    setSelectedFile(file);
    setStatus(null);
  }, []);

  const handleDragOver = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    setIsDragging(true);
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    setIsDragging(false);
  }, []);

  const handleDrop = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    setIsDragging(false);
    handleFile(e.dataTransfer.files[0]);
  }, [handleFile]);

  const handleFileInput = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    handleFile(e.target.files?.[0]);
  }, [handleFile]);

  const handleUpload = async (): Promise<void> => {
    if (!selectedFile) return;

    try {
      setUploading(true);
      setStatus({ type: 'loading', message: 'Getting upload URL...' });

      // Simulate initial connection delay
      await new Promise(resolve => setTimeout(resolve, 300));
      setProgress(5);

      const urlResponse = await fetch(API_ENDPOINT, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' }
      });

      if (!urlResponse.ok) {
        throw new Error('Failed to get upload URL');
      }

      const { uploadUrl, fileName: s3FileName }: UploadResponse = await urlResponse.json();
      setProgress(15);

      setStatus({ type: 'loading', message: 'Uploading file...' });

      // Simulate realistic upload progress based on file size
      const fileSize = selectedFile.size;
      const chunks = Math.ceil(fileSize / (1024 * 1024)); // Chunks of 1MB
      const baseDelay = 200; // Base delay per chunk

      // Simulate chunked upload with progress
      const uploadPromise = fetch(uploadUrl, {
        method: 'PUT',
        body: selectedFile,
        headers: { 'Content-Type': 'text/csv' }
      });

      // Simulate progress during upload
      let currentProgress = 15;
      const targetProgress = 85;
      const progressInterval = setInterval(() => {
        if (currentProgress < targetProgress) {
          // Slower progress as we get closer to completion (realistic behavior)
          const increment = Math.max(1, (targetProgress - currentProgress) / 10);
          currentProgress = Math.min(targetProgress, currentProgress + increment);
          setProgress(Math.floor(currentProgress));
        }
      }, chunks > 5 ? 400 : 200); // Slower for larger files

      const uploadResponse = await uploadPromise;
      clearInterval(progressInterval);

      if (!uploadResponse.ok) {
        throw new Error('Failed to upload file');
      }

      setProgress(90);
      setStatus({ type: 'loading', message: 'Verifying upload...' });
      await new Promise(resolve => setTimeout(resolve, 500));

      setProgress(95);
      setStatus({ type: 'loading', message: 'Processing data...' });
      await new Promise(resolve => setTimeout(resolve, 800));

      setProgress(100);
      setStatus({
        type: 'success',
        message: `Upload successful! File: ${s3FileName}. Processing started. Users will be matched with loan products shortly.`
      });

      setTimeout(() => {
        setSelectedFile(null);
        setProgress(0);
        setStatus(null);
      }, 3000);
    } catch (error) {
      console.error('Upload error:', error);
      setStatus({
        type: 'error',
        message: error instanceof Error ? error.message : 'An error occurred'
      });
      setProgress(0);
    } finally {
      setUploading(false);
    }
  };

  return (
    <div className="min-h-screen bg-background flex items-center justify-center p-4">
      <Card className="w-full max-w-2xl">
        <CardHeader>
          <CardTitle className="text-3xl flex items-center gap-2">
            🏦 Loan Eligibility System
          </CardTitle>
          <CardDescription>
            Upload CSV file with user data
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
            onClick={() => document.getElementById('fileInput').click()}
            className={`border-2 border-dashed rounded-lg p-12 text-center cursor-pointer transition-all ${
              isDragging
                ? 'border-primary bg-accent scale-105'
                : 'border-muted-foreground/25 hover:border-muted-foreground/50 hover:bg-accent/50'
            }`}
          >
            <Upload className="w-12 h-12 mx-auto mb-4 text-muted-foreground" />
            <p className="text-lg font-semibold mb-2">
              Click to upload or drag and drop
            </p>
            <p className="text-sm text-muted-foreground">
              CSV file (user_id, email, monthly_income, credit_score, employment_status, age)
            </p>
            <input
              type="file"
              id="fileInput"
              accept=".csv"
              className="hidden"
              onChange={handleFileInput}
              disabled={uploading}
            />
          </div>

          {selectedFile && (
            <div className="bg-muted rounded-lg p-4 flex items-center gap-3">
              <FileText className="w-8 h-8 text-muted-foreground" />
              <div className="flex-1">
                <p className="font-semibold">{selectedFile.name}</p>
                <p className="text-sm text-muted-foreground">{formatFileSize(selectedFile.size)}</p>
              </div>
            </div>
          )}

          <Button
            onClick={handleUpload}
            disabled={!selectedFile || uploading}
            className="w-full"
            size="lg"
          >
            {uploading ? (
              <>
                <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                Uploading...
              </>
            ) : (
              'Upload File'
            )}
          </Button>

          {progress > 0 && progress < 100 && (
            <Progress value={progress} className="w-full" />
          )}

          {status && (
            <Alert variant={status.type === 'error' ? 'destructive' : 'default'}>
              {status.type === 'success' && <CheckCircle2 className="h-4 w-4" />}
              {status.type === 'error' && <AlertCircle className="h-4 w-4" />}
              {status.type === 'loading' && <Loader2 className="h-4 w-4 animate-spin" />}
              <AlertDescription>{status.message}</AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
