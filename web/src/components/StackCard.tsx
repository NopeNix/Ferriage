import { Container, FolderOpen, Image as ImageIcon } from "lucide-react";
import { Badge } from "./ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import type { Stack } from "../api/types";

function shortImage(image: string): string {
  const name = image.split("/").pop() ?? image;
  return name.split(":")[0];
}

export function StackCard({ stack, onOpen }: { stack: Stack; onOpen?: () => void }) {
  const healthy = stack.running_count === stack.container_count && stack.container_count > 0;
  const body = (
    <Card data-testid="stack-card">
      <CardHeader>
        <div className="flex items-center justify-between gap-2">
          <CardTitle className="flex items-center gap-2">
            <Container className="size-5" />
            <span data-testid="stack-name">{stack.name}</span>
          </CardTitle>
          <Badge variant={healthy ? "default" : "secondary"}>
            {stack.running_count}/{stack.container_count} running
          </Badge>
        </div>
        {stack.compose_file && (
          <CardDescription className="flex items-center gap-1.5">
            <FolderOpen className="size-3.5" />
            <span className="truncate">{stack.compose_file}</span>
          </CardDescription>
        )}
      </CardHeader>
      <CardContent>
        <ul className="space-y-2">
          {stack.services.map((s) => (
            <li key={`${s.name}-${s.container}`} className="flex items-center justify-between gap-2 text-sm">
              <span className="flex items-center gap-2">
                <span
                  className={`inline-block size-2 rounded-full ${s.running ? "bg-emerald-500" : "bg-destructive"}`}
                />
                <span className="font-medium">{s.name}</span>
                <span className="text-muted-foreground">{s.container}</span>
              </span>
              <span className="flex items-center gap-1 text-muted-foreground">
                <ImageIcon className="size-3.5" />
                <span className="truncate">{shortImage(s.image)}</span>
              </span>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
  if (!onOpen) return body;
  return (
    <button type="button" className="block w-full text-left" data-testid="stack-card-wrap" onClick={onOpen}>
      {body}
    </button>
  );
}
